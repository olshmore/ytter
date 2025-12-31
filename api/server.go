package api

import (
	"fmt"

	db "github.com/olshmore/ytter/db/sqlc"
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
}

func NewServer(config config.Config, store db.Store, taskDistributor worker.TaskDistributor) (*Server, error) {
	tokenMaker, err := token.NewPasetoMaker(config.TokenSymmetricKey)
	if err != nil {
		return nil, fmt.Errorf("cannot create token maker: %w", err)
	}

	server := &Server{
		config:          config,
		store:           store,
		tokenMaker:      tokenMaker,
		taskDistributor: taskDistributor,
	}

	return server, nil
}
