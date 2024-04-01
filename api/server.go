package api

import (
	"github.com/olshmore/ytter/config"
	db "github.com/olshmore/ytter/db/sqlc"
	"github.com/olshmore/ytter/pb"
)

type Server struct {
	pb.UnimplementedYtterServer
	config config.Config
	store  db.Store
}

func NewServer(config config.Config, store db.Store) (*Server, error) {
	server := &Server{
		config: config,
		store:  store,
	}

	return server, nil
}
