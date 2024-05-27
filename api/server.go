package api

import (
	"github.com/olshmore/ytter/config"
	db "github.com/olshmore/ytter/db/sqlc"
	"github.com/olshmore/ytter/pb"
	"github.com/olshmore/ytter/token"
	"github.com/rs/zerolog/log"
)

type Server struct {
	pb.UnimplementedYtterServer
	config     config.Config
	store      db.Store
	tokenMaker token.Maker
}

func NewServer(config config.Config, store db.Store) (*Server, error) {
	tokenMaker, err := token.NewPasetoMaker(config.TokenSymmetricKey)
	if err != nil {
		log.Error().Msgf("cannot create token maker: %w", err)

		return nil, err
	}

	server := &Server{
		config:     config,
		store:      store,
		tokenMaker: tokenMaker,
	}

	return server, nil
}
