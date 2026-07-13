package api

import (
	"fmt"

	db "github.com/olshmore/ytter/db/sqlc"
	"github.com/olshmore/ytter/internal/ai"
	"github.com/olshmore/ytter/internal/storage"
	"github.com/olshmore/ytter/internal/worker"
	"github.com/olshmore/ytter/pb"
	"github.com/olshmore/ytter/pkg/config"
	"github.com/olshmore/ytter/pkg/token"
)

type Server struct {
	pb.UnimplementedYtterServer
	config          config.Config
	store           db.Store
	tokenMaker      token.Maker
	taskDistributor worker.TaskDistributor
	aiGateway       ai.Gateway
	hostSlotPlans   *hostSlotPlanStore
	objectStore     storage.ObjectStore
}

func NewServer(config config.Config, store db.Store, taskDistributor worker.TaskDistributor) (*Server, error) {
	tokenMaker, err := token.NewPasetoMaker(config.TokenSymmetricKey)
	if err != nil {
		return nil, fmt.Errorf("cannot create token maker: %w", err)
	}

	objectStore, err := storage.NewFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("cannot create object store: %w", err)
	}

	server := &Server{
		config:          config,
		store:           store,
		tokenMaker:      tokenMaker,
		taskDistributor: taskDistributor,
		aiGateway:       ai.NewGatewayFromConfig(config),
		hostSlotPlans:   newHostSlotPlanStore(),
		objectStore:     objectStore,
	}

	return server, nil
}

// AIGateway returns the configured AI gateway for the server.
func (s *Server) AIGateway() ai.Gateway {
	return s.aiGateway
}
