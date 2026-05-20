package api

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/olshmore/ytter/db/sqlc"
	"github.com/olshmore/ytter/pb"
)

const (
	maxHostSlotPreviewRows   = 200
	maxHostSlotPlanTotalOps  = 2000
	minGapWarningMinutes     = 5
)

type resolvedBatchSlotPlan struct {
	ServiceID        uuid.UUID
	ServiceName      string
	PractitionerID   pgtype.UUID
	PractitionerName string
	RoomID           pgtype.UUID
	RoomName         string
	DateFrom         time.Time
	DateTo           time.Time
	Weekdays         map[string]struct{}
	DailyStart       time.Duration
	DailyEnd         time.Duration
	SlotMinutes      int32
	Capacity         int32
	Status           string
}

type slotTimeInterval struct {
	Start time.Time
	End   time.Time
}

func resourceLabel(practitionerName, roomName string) string {
	parts := make([]string, 0, 2)
	if strings.TrimSpace(practitionerName) != "" {
		parts = append(parts, practitionerName)
	}
	if strings.TrimSpace(roomName) != "" {
		parts = append(parts, roomName)
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, " · ")
}

func normalizeWeekdayToken(day string) string {
	d := strings.ToLower(strings.TrimSpace(day))
	switch {
	case strings.HasPrefix(d, "mon"):
		return "mon"
	case strings.HasPrefix(d, "tue"):
		return "tue"
	case strings.HasPrefix(d, "wed"):
		return "wed"
	case strings.HasPrefix(d, "thu"):
		return "thu"
	case strings.HasPrefix(d, "fri"):
		return "fri"
	case strings.HasPrefix(d, "sat"):
		return "sat"
	case strings.HasPrefix(d, "sun"):
		return "sun"
	default:
		return ""
	}
}

func weekdayKeyFromTime(t time.Time) string {
	return strings.ToLower(t.Weekday().String()[:3])
}

// formatUKDateFromISO renders YYYY-MM-DD for host-facing copy (e.g. 17 May 2026).
func formatUKDateFromISO(iso string) string {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(iso))
	if err != nil {
		return iso
	}
	return t.Format("2 Jan 2006")
}

// validateBatchDateStringsNotBeforeToday rejects calendar ranges that end before
// today in the location timezone. ISO date strings compare lexicographically.
func validateBatchDateStringsNotBeforeToday(dateFrom, dateTo, todayLocal string) ([]string, bool) {
	from := strings.TrimSpace(dateFrom)
	to := strings.TrimSpace(dateTo)
	today := strings.TrimSpace(todayLocal)
	if from == "" || to == "" || today == "" {
		return nil, false
	}
	var msgs []string
	if from < today {
		msgs = append(msgs, fmt.Sprintf("date_from must be on or after today (%s)", formatUKDateFromISO(today)))
	}
	if to < today {
		msgs = append(msgs, fmt.Sprintf("date_to must be on or after today (%s)", formatUKDateFromISO(today)))
	}
	return msgs, len(msgs) > 0
}

func intervalsOverlap(a, b slotTimeInterval) bool {
	return a.Start.Before(b.End) && b.Start.Before(a.End)
}

func expandBatchSlotPlan(
	plan resolvedBatchSlotPlan,
	existing []slotTimeInterval,
) (ops []*pb.HostSlotAssistantPlanOperation, validation []string, total int, hasBlocking bool) {
	if plan.SlotMinutes <= 0 || plan.Capacity <= 0 {
		validation = append(validation, "slot_minutes and capacity must be greater than zero")
		hasBlocking = true
		return nil, validation, 0, hasBlocking
	}
	if plan.DailyEnd <= plan.DailyStart {
		validation = append(validation, "daily_end_local must be after daily_start_local")
		hasBlocking = true
		return nil, validation, 0, hasBlocking
	}
	if len(plan.Weekdays) == 0 {
		validation = append(validation, "select at least one weekday")
		hasBlocking = true
		return nil, validation, 0, hasBlocking
	}

	step := time.Duration(plan.SlotMinutes) * time.Minute
	label := resourceLabel(plan.PractitionerName, plan.RoomName)
	proposed := make([]slotTimeInterval, 0, 64)
	now := time.Now()
	futureCount := 0

	for d := plan.DateFrom; !d.After(plan.DateTo); d = d.AddDate(0, 0, 1) {
		dayKey := weekdayKeyFromTime(d)
		if _, ok := plan.Weekdays[dayKey]; !ok {
			continue
		}
		dayStart := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC).Add(plan.DailyStart)
		dayEnd := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC).Add(plan.DailyEnd)
		for st := dayStart; st.Add(step).Equal(dayEnd) || st.Add(step).Before(dayEnd); st = st.Add(step) {
			et := st.Add(step)
			total++
			if total > maxHostSlotPlanTotalOps {
				validation = append(validation, fmt.Sprintf("plan would create more than %d slots; narrow the date range or hours", maxHostSlotPlanTotalOps))
				hasBlocking = true
				return ops, validation, total, hasBlocking
			}

			interval := slotTimeInterval{Start: st, End: et}
			if st.After(now) {
				futureCount++
			}
			level := "valid"
			note := ""
			for _, ex := range existing {
				if intervalsOverlap(interval, ex) {
					level = "error"
					note = "Overlaps an existing slot"
					hasBlocking = true
					break
				}
			}
			if level == "valid" && len(proposed) > 0 {
				prev := proposed[len(proposed)-1]
				gap := st.Sub(prev.End)
				if gap >= 0 && gap < minGapWarningMinutes*time.Minute {
					level = "warning"
					note = "Short buffer between back-to-back sessions"
				}
			}
			proposed = append(proposed, interval)

			if len(ops) < maxHostSlotPreviewRows {
				when := formatSlotWhenLocal(st, et)
				ops = append(ops, &pb.HostSlotAssistantPlanOperation{
					StartAt:       when,
					EndAt:         et.Format(time.RFC3339),
					ServiceName:   plan.ServiceName,
					ResourceLabel: label,
					Status:        level,
					Note:          note,
				})
			}
		}
	}

	if total == 0 {
		validation = append(validation, "no slots match the selected weekdays and date range")
		hasBlocking = true
	}
	if total > 0 && futureCount == 0 {
		validation = append(validation, "all slots in this plan are in the past; guests will not see them on the public booking page")
		hasBlocking = true
	}
	if total > maxHostSlotPreviewRows {
		validation = append(validation, fmt.Sprintf("showing first %d of %d slots in preview", maxHostSlotPreviewRows, total))
	}
	return ops, validation, total, hasBlocking
}

func formatSlotWhenLocal(start, end time.Time) string {
	return start.Format("Mon 2 Jan 2006, 15:04") + "–" + end.Format("15:04")
}

func loadExistingSlotIntervals(rows []db.ListHostSlotsByLocationRow) []slotTimeInterval {
	out := make([]slotTimeInterval, 0, len(rows))
	for _, row := range rows {
		out = append(out, slotTimeInterval{Start: row.StartAt, End: row.EndAt})
	}
	return out
}

func resolveServiceByName(services []db.ListHostServicesByLocationRow, name string) (db.ListHostServicesByLocationRow, bool) {
	want := strings.TrimSpace(strings.ToLower(name))
	if want == "" {
		return db.ListHostServicesByLocationRow{}, false
	}
	var partial db.ListHostServicesByLocationRow
	partialOK := false
	for _, s := range services {
		n := strings.ToLower(strings.TrimSpace(s.Name))
		if n == want {
			return s, true
		}
		if strings.Contains(n, want) || strings.Contains(want, n) {
			if !partialOK {
				partial = s
				partialOK = true
			}
		}
	}
	if partialOK {
		return partial, true
	}
	return db.ListHostServicesByLocationRow{}, false
}
