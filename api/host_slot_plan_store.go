package api

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/olshmore/ytter/pb"
)

const (
	hostSlotPlanTTL        = 30 * time.Minute
	maxHostSlotPlanEntries = 256
)

type storedHostSlotPlan struct {
	planID           string
	locationSlug     string
	ownerUsername    string
	batch            *pb.CreateHostLocationSlotsBatchRequest
	hasBlocking      bool
	createdAt        time.Time
	expiresAt        time.Time
}

type hostSlotPlanStore struct {
	mu sync.RWMutex
	// planID -> plan
	plans map[string]*storedHostSlotPlan
	// idempotencyKey -> cached publish response
	publishCache map[string]*pb.HostSlotAssistantPublishResponse
}

func newHostSlotPlanStore() *hostSlotPlanStore {
	return &hostSlotPlanStore{
		plans:        make(map[string]*storedHostSlotPlan),
		publishCache: make(map[string]*pb.HostSlotAssistantPublishResponse),
	}
}

func (s *hostSlotPlanStore) put(plan *storedHostSlotPlan) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictExpiredLocked()
	if len(s.plans) >= maxHostSlotPlanEntries {
		s.evictOldestLocked()
	}
	s.plans[plan.planID] = plan
}

func (s *hostSlotPlanStore) get(planID, locationSlug, ownerUsername string) (*storedHostSlotPlan, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.plans[planID]
	if !ok || time.Now().After(p.expiresAt) {
		return nil, false
	}
	if p.locationSlug != locationSlug || p.ownerUsername != ownerUsername {
		return nil, false
	}
	return p, true
}

func (s *hostSlotPlanStore) delete(planID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.plans, planID)
}

func (s *hostSlotPlanStore) getPublishCache(idempotencyKey string) (*pb.HostSlotAssistantPublishResponse, bool) {
	if idempotencyKey == "" {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	res, ok := s.publishCache[idempotencyKey]
	return res, ok
}

func (s *hostSlotPlanStore) putPublishCache(idempotencyKey string, res *pb.HostSlotAssistantPublishResponse) {
	if idempotencyKey == "" || res == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.publishCache[idempotencyKey] = res
}

func (s *hostSlotPlanStore) evictExpiredLocked() {
	now := time.Now()
	for id, p := range s.plans {
		if now.After(p.expiresAt) {
			delete(s.plans, id)
		}
	}
}

func (s *hostSlotPlanStore) evictOldestLocked() {
	var oldestID string
	var oldest time.Time
	for id, p := range s.plans {
		if oldestID == "" || p.createdAt.Before(oldest) {
			oldestID = id
			oldest = p.createdAt
		}
	}
	if oldestID != "" {
		delete(s.plans, oldestID)
	}
}

func newStoredHostSlotPlan(
	locationSlug, ownerUsername string,
	batch *pb.CreateHostLocationSlotsBatchRequest,
	hasBlocking bool,
) *storedHostSlotPlan {
	now := time.Now()
	return &storedHostSlotPlan{
		planID:        uuid.NewString(),
		locationSlug:  locationSlug,
		ownerUsername: ownerUsername,
		batch:         batch,
		hasBlocking:   hasBlocking,
		createdAt:     now,
		expiresAt:     now.Add(hostSlotPlanTTL),
	}
}
