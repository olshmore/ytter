package api

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/olshmore/ytter/db/sqlc"
	"github.com/olshmore/ytter/pb"
	"github.com/olshmore/ytter/pkg/utils"
	"golang.org/x/oauth2"
	googleOAuth2 "golang.org/x/oauth2/google"
	googleOAuth2API "google.golang.org/api/oauth2/v2"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (server *Server) InitiateGoogleAuth(ctx context.Context, req *pb.InitiateGoogleAuthRequest) (*pb.InitiateGoogleAuthResponse, error) {
	if server.config.GoogleClientID == "" || server.config.GoogleClientSecret == "" {
		return nil, status.Errorf(codes.Internal, "Google OAuth is not configured")
	}

	// Generate state token for CSRF protection
	state, err := utils.RandomStateToken()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate state token")
	}

	// Configure OAuth2
	config := &oauth2.Config{
		ClientID:     server.config.GoogleClientID,
		ClientSecret: server.config.GoogleClientSecret,
		RedirectURL:  server.config.GoogleRedirectURL,
		Scopes: []string{
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		},
		Endpoint: googleOAuth2.Endpoint,
	}

	// Generate auth URL
	authURL := config.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)

	return &pb.InitiateGoogleAuthResponse{
		AuthUrl: authURL,
	}, nil
}

func (server *Server) GoogleAuthCallback(ctx context.Context, req *pb.GoogleAuthCallbackRequest) (*pb.GoogleAuthCallbackResponse, error) {
	if server.config.GoogleClientID == "" || server.config.GoogleClientSecret == "" {
		return nil, status.Errorf(codes.Internal, "Google OAuth is not configured")
	}

	// Validate state token
	if req.GetState() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "state parameter is required")
	}

	// Configure OAuth2
	config := &oauth2.Config{
		ClientID:     server.config.GoogleClientID,
		ClientSecret: server.config.GoogleClientSecret,
		RedirectURL:  server.config.GoogleRedirectURL,
		Scopes: []string{
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		},
		Endpoint: googleOAuth2.Endpoint,
	}

	// Exchange code for token
	token, err := config.Exchange(ctx, req.GetCode())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to exchange code for token: %v", err)
	}

	// Get user info from Google
	oauth2Service, err := googleOAuth2API.NewService(ctx, option.WithTokenSource(config.TokenSource(ctx, token)))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create OAuth2 service: %v", err)
	}

	userInfo, err := oauth2Service.Userinfo.Get().Do()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user info: %v", err)
	}

	googleID := userInfo.Id
	email := userInfo.Email
	firstName := userInfo.GivenName
	lastName := userInfo.FamilyName

	if googleID == "" || email == "" {
		return nil, status.Errorf(codes.Internal, "invalid user info from Google")
	}

	// Check if user exists with this Google ID
	googleIDText := pgtype.Text{String: googleID, Valid: true}
	user, err := server.store.GetUserByGoogleID(ctx, googleIDText)
	if err != nil {
		if !errors.Is(err, db.ErrRecordNotFound) {
			return nil, status.Errorf(codes.Internal, "failed to check user: %v", err)
		}

		// User doesn't exist with Google ID, check if email exists
		existingUser, err := server.store.GetUserByEmail(ctx, email)
		if err != nil && !errors.Is(err, db.ErrRecordNotFound) {
			return nil, status.Errorf(codes.Internal, "failed to check email: %v", err)
		}

		if errors.Is(err, db.ErrRecordNotFound) {
			// Create new user with Google OAuth
			// Generate username from email (part before @)
			username := generateUsernameFromEmail(email)

			// Generate a random password (user won't use it for Google OAuth)
			randomPassword, err := utils.RandomPassword()
			if err != nil {
				return nil, status.Errorf(codes.Internal, "failed to generate password: %v", err)
			}
			hashedPassword, err := utils.HashPassword(randomPassword)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "failed to hash password: %v", err)
			}

			createUserParams := db.CreateUserParams{
				Username:       username,
				HashedPassword: hashedPassword,
				FirstName:      firstName,
				LastName:       lastName,
				Email:          email,
				Role:           "member",
			}

			// TODO: Update CreateUser to support google_id
			// For now, create user then update with google_id
			user, err = server.store.CreateUser(ctx, createUserParams)
			if err != nil {
				if db.ErrorCode(err) == db.UniqueViolation {
					// Username or email already exists, try with different username
					username = generateUsernameFromEmail(email) + "_" + utils.RandomAlphaNumericString(6)
					createUserParams.Username = username
					user, err = server.store.CreateUser(ctx, createUserParams)
					if err != nil {
						return nil, status.Errorf(codes.Internal, "failed to create user: %v", err)
					}
				} else {
					return nil, status.Errorf(codes.Internal, "failed to create user: %v", err)
				}
			}

			// Update user with Google ID
			googleIDText := pgtype.Text{String: googleID, Valid: true}
			updateParams := db.UpdateUserParams{
				Username: user.Username,
				GoogleID: googleIDText,
			}
			user, err = server.store.UpdateUser(ctx, updateParams)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "failed to update user with Google ID: %v", err)
			}
		} else {
			// User exists with this email, link Google ID to existing account
			googleIDText := pgtype.Text{String: googleID, Valid: true}
			updateParams := db.UpdateUserParams{
				Username: existingUser.Username,
				GoogleID: googleIDText,
			}
			user, err = server.store.UpdateUser(ctx, updateParams)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "failed to link Google ID: %v", err)
			}
		}
	}

	// Generate tokens (same as login)
	accessToken, accessPayload, err := server.tokenMaker.CreateToken(
		user.Username,
		utils.Role(user.Role),
		server.config.AccessTokenDuration,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create access token")
	}

	refreshToken, refreshPayload, err := server.tokenMaker.CreateToken(
		user.Username,
		utils.Role(user.Role),
		server.config.RefreshTokenDuration,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create refresh token")
	}

	mtdt := server.extractMetadata(ctx)
	session, err := server.store.CreateSession(ctx, db.CreateSessionParams{
		ID:           refreshPayload.ID,
		Username:     user.Username,
		RefreshToken: refreshToken,
		UserAgent:    mtdt.UserAgent,
		ClientIp:     mtdt.ClientIP,
		IsBlocked:    false,
		ExpiresAt:    refreshPayload.ExpiredAt,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create session")
	}

	res := &pb.GoogleAuthCallbackResponse{
		User:                  ConvertUser(user),
		SessionId:             session.ID.String(),
		AccessToken:           accessToken,
		RefreshToken:          refreshToken,
		AccessTokenExpiresAt:  timestamppb.New(accessPayload.ExpiredAt),
		RefreshTokenExpiresAt: timestamppb.New(refreshPayload.ExpiredAt),
	}

	return res, nil
}

// Helper functions
func generateUsernameFromEmail(email string) string {
	parts := strings.Split(email, "@")
	username := strings.ToLower(parts[0])
	// Remove any special characters
	username = strings.ReplaceAll(username, ".", "_")
	username = strings.ReplaceAll(username, "+", "_")
	return username
}
