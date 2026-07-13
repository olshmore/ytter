package api

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/olshmore/ytter/db/sqlc"
	"github.com/olshmore/ytter/internal/booking/access"
	"github.com/olshmore/ytter/internal/storage"
	"github.com/olshmore/ytter/pb"
	"github.com/olshmore/ytter/pkg/utils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	brandingHexColor = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	brandingFonts    = map[string]struct{}{
		"public_sans":    {},
		"source_sans_3":  {},
		"lora":           {},
		"source_serif_4": {},
		"ibm_plex_sans":  {},
		"dm_sans":        {},
	}
)

func optionalTextPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	s := t.String
	return &s
}

func applyLocationBrandingFields(dst *pb.HostLocationItem, loc db.Location) {
	dst.LogoUrl = optionalTextPtr(loc.LogoUrl)
	dst.PrimaryColor = optionalTextPtr(loc.PrimaryColor)
	dst.AccentColor = optionalTextPtr(loc.AccentColor)
	dst.BackgroundColor = optionalTextPtr(loc.BackgroundColor)
	dst.FontFamily = optionalTextPtr(loc.FontFamily)
}

func normalizeHexColor(raw string) (string, error) {
	c := strings.TrimSpace(raw)
	if !brandingHexColor.MatchString(c) {
		return "", status.Errorf(codes.InvalidArgument, "color must be #RRGGBB")
	}
	return strings.ToUpper(c), nil
}

func validateFontFamily(raw string) (string, error) {
	f := strings.TrimSpace(raw)
	if _, ok := brandingFonts[f]; !ok {
		return "", status.Errorf(codes.InvalidArgument, "invalid font_family")
	}
	return f, nil
}

func brandingResponseFromLocation(loc db.Location) *pb.UpdateLocationBrandingResponse {
	return &pb.UpdateLocationBrandingResponse{
		LocationId:    loc.ID.String(),
		LocationSlug:  loc.Slug,
		LocationName:  loc.Name,
		LogoUrl:          optionalTextPtr(loc.LogoUrl),
		PrimaryColor:     optionalTextPtr(loc.PrimaryColor),
		AccentColor:      optionalTextPtr(loc.AccentColor),
		BackgroundColor:  optionalTextPtr(loc.BackgroundColor),
		FontFamily:       optionalTextPtr(loc.FontFamily),
	}
}

func (server *Server) UpdateLocationBranding(ctx context.Context, req *pb.UpdateLocationBrandingRequest) (*pb.UpdateLocationBrandingResponse, error) {
	payload, err := server.MustGetAuthPayload(ctx, []utils.Role{utils.RoleHost, utils.RoleAdmin})
	if err != nil {
		return nil, unauthenticatedError(err)
	}
	locationSlug := strings.TrimSpace(req.GetLocationSlug())
	if locationSlug == "" {
		return nil, status.Errorf(codes.InvalidArgument, "location_slug is required")
	}
	current, err := server.store.GetLocationBySlug(ctx, locationSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, status.Errorf(codes.NotFound, "location not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to load location")
	}
	if !access.HostMayAccessLocation(payload, current.OwnerUsername) {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}

	arg := db.UpdateLocationBrandingParams{ID: current.ID}
	if req.ResetAll != nil && req.GetResetAll() {
		arg.ResetBranding = true
	} else {
		if req.LogoUrl != nil {
			logo := strings.TrimSpace(req.GetLogoUrl())
			if logo == "" {
				arg.ClearLogoUrl = true
			} else {
				if server.objectStore == nil || !server.objectStore.IsPlatformURL(logo) {
					return nil, status.Errorf(codes.InvalidArgument, "logo_url must be a platform upload URL")
				}
				arg.SetLogoUrl = true
				arg.LogoUrl = pgtype.Text{String: logo, Valid: true}
			}
		}
		if req.PrimaryColor != nil {
			color := strings.TrimSpace(req.GetPrimaryColor())
			if color == "" {
				arg.ClearPrimaryColor = true
			} else {
				normalized, err := normalizeHexColor(color)
				if err != nil {
					return nil, err
				}
				arg.SetPrimaryColor = true
				arg.PrimaryColor = pgtype.Text{String: normalized, Valid: true}
			}
		}
		if req.AccentColor != nil {
			color := strings.TrimSpace(req.GetAccentColor())
			if color == "" {
				arg.ClearAccentColor = true
			} else {
				normalized, err := normalizeHexColor(color)
				if err != nil {
					return nil, err
				}
				arg.SetAccentColor = true
				arg.AccentColor = pgtype.Text{String: normalized, Valid: true}
			}
		}
		if req.BackgroundColor != nil {
			color := strings.TrimSpace(req.GetBackgroundColor())
			if color == "" {
				arg.ClearBackgroundColor = true
			} else {
				normalized, err := normalizeHexColor(color)
				if err != nil {
					return nil, err
				}
				arg.SetBackgroundColor = true
				arg.BackgroundColor = pgtype.Text{String: normalized, Valid: true}
			}
		}
		if req.FontFamily != nil {
			font := strings.TrimSpace(req.GetFontFamily())
			if font == "" {
				arg.ClearFontFamily = true
			} else {
				normalized, err := validateFontFamily(font)
				if err != nil {
					return nil, err
				}
				arg.SetFontFamily = true
				arg.FontFamily = pgtype.Text{String: normalized, Valid: true}
			}
		}
	}

	loc, err := server.store.UpdateLocationBranding(ctx, arg)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update branding")
	}
	return brandingResponseFromLocation(loc), nil
}

func (server *Server) CreateLocationBrandingLogoUpload(ctx context.Context, req *pb.CreateLocationBrandingLogoUploadRequest) (*pb.CreateLocationBrandingLogoUploadResponse, error) {
	payload, err := server.MustGetAuthPayload(ctx, []utils.Role{utils.RoleHost, utils.RoleAdmin})
	if err != nil {
		return nil, unauthenticatedError(err)
	}
	locationSlug := strings.TrimSpace(req.GetLocationSlug())
	if locationSlug == "" {
		return nil, status.Errorf(codes.InvalidArgument, "location_slug is required")
	}
	current, err := server.store.GetLocationBySlug(ctx, locationSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, status.Errorf(codes.NotFound, "location not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to load location")
	}
	if !access.HostMayAccessLocation(payload, current.OwnerUsername) {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}

	contentType := strings.TrimSpace(req.GetContentType())
	if _, ok := storage.ExtForContentType(contentType); !ok {
		return nil, status.Errorf(codes.InvalidArgument, "unsupported content_type")
	}
	contentLength := req.GetContentLength()
	if contentLength <= 0 || contentLength > storage.MaxLogoBytes {
		return nil, status.Errorf(codes.InvalidArgument, "content_length must be between 1 and %d", storage.MaxLogoBytes)
	}
	if server.objectStore == nil || !server.objectStore.Configured() {
		return nil, status.Errorf(codes.FailedPrecondition, "logo upload temporarily unavailable")
	}

	result, err := server.objectStore.PresignPut(ctx, current.ID, contentType, contentLength)
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "logo upload temporarily unavailable")
	}
	return &pb.CreateLocationBrandingLogoUploadResponse{
		UploadUrl: result.UploadURL,
		PublicUrl: result.PublicURL,
		ExpiresAt: result.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z"),
	}, nil
}
