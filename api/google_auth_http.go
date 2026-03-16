package api

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc/status"

	"github.com/olshmore/ytter/pb"
)

// GoogleAuthCallbackHTTPHandler returns an HTTP handler that:
//   - accepts Google's /v1/auth/google/callback redirect with code & state
//   - delegates to the gRPC GoogleAuthCallback method
//   - redirects the browser to the SPA with tokens and user info in the query string
func (server *Server) GoogleAuthCallbackHTTPHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")

		if code == "" || state == "" {
			http.Error(w, "missing code or state", http.StatusBadRequest)
			return
		}

		res, err := server.GoogleAuthCallback(r.Context(), &pb.GoogleAuthCallbackRequest{
			Code:  code,
			State: state,
		})
		if err != nil {
			st, ok := status.FromError(err)
			if !ok {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}

			http.Error(w, st.Message(), runtime.HTTPStatusFromCode(st.Code()))
			return
		}

		frontendBase := server.config.FrontendBaseURL
		if frontendBase == "" {
			if len(server.config.AllowedOrigins) > 0 {
				frontendBase = server.config.AllowedOrigins[0]
			} else {
				frontendBase = "http://localhost:3000"
			}
		}

		u, err := url.Parse(frontendBase)
		if err != nil {
			http.Error(w, "invalid frontend base url", http.StatusInternalServerError)
			return
		}
		u.Path = "/auth/callback"

		q := u.Query()
		q.Set("accessToken", res.AccessToken)
		q.Set("refreshToken", res.RefreshToken)
		q.Set("sessionId", res.SessionId)

		if res.User != nil {
			if data, err := json.Marshal(res.User); err == nil {
				q.Set("user", string(data))
			}
		}

		u.RawQuery = q.Encode()

		http.Redirect(w, r, u.String(), http.StatusFound)
	}
}

