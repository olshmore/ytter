package api

import (
	"fmt"

	"github.com/olshmore/ytter/pb"
	"github.com/olshmore/ytter/pkg/utils"
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Route method names from generated proto code
const (
	RouteCreateUser   = pb.Ytter_CreateUser_FullMethodName
	RouteVerifyEmail  = pb.Ytter_VerifyEmail_FullMethodName
	RouteLoginUser    = pb.Ytter_LoginUser_FullMethodName
	RouteRefreshToken = pb.Ytter_RefreshToken_FullMethodName
	RouteListUsers    = pb.Ytter_ListUsers_FullMethodName
	RouteUpdateUser   = pb.Ytter_UpdateUser_FullMethodName
)

// ConfigureRoleBasedAccess sets up GRPC role-based access control
func ConfigureRoleBasedAccess() RoleConfig {
	config := NewRoleConfig()

	// ============================================================================
	// UNAUTHENTICATED
	// Routes accessible without authentication
	// ============================================================================
	// No configuration needed - routes not listed here are public by default
	_ = []string{
		RouteCreateUser,   // registration
		RouteLoginUser,    // login
		RouteVerifyEmail,  // email verification
		RouteRefreshToken, // token refresh
	}

	// ============================================================================
	// AUTHENTICATED: MEMBER OR ADMIN
	// Routes accessible by authenticated members or admins
	// ============================================================================
	config.RequireAuth([]utils.Role{utils.RoleMember, utils.RoleAdmin},
		RouteUpdateUser,
	)

	// ============================================================================
	// AUTHENTICATED: ADMIN ONLY
	// Routes accessible only by authenticated admin users
	// ============================================================================
	config.RequireAdminRole(
		RouteListUsers,
	)

	// ============================================================================
	// AUTHENTICATED: MEMBER ONLY
	// Routes accessible only by authenticated members (not admin)
	// ============================================================================
	// config.RequireMemberRole(
	// )

	return config
}

// HTTPPathToGRPCMethodMap builds a mapping from HTTP paths to gRPC method names
func HTTPPathToGRPCMethodMap() (map[string]string, error) {
	mapping := make(map[string]string)

	fileDesc := pb.File_service_ytter_proto
	if fileDesc == nil {
		return nil, fmt.Errorf("proto file descriptor not available")
	}

	serviceDesc := fileDesc.Services().ByName("Ytter")
	if serviceDesc == nil {
		return nil, fmt.Errorf("service descriptor not found")
	}

	serviceFullName := string(serviceDesc.FullName())

	methods := serviceDesc.Methods()
	for i := 0; i < methods.Len(); i++ {
		methodDesc := methods.Get(i)

		// full method name: "/{package}.{Service}/{Method}"
		fullMethodName := fmt.Sprintf("/%s/%s", serviceFullName, methodDesc.Name())

		path := extractHTTPPathFromMethod(methodDesc)
		if path == "" {
			return nil, fmt.Errorf("HTTP path not found for method %s (full method: %s)", methodDesc.Name(), fullMethodName)
		}

		mapping[path] = fullMethodName
	}

	return mapping, nil
}

func extractHTTPPathFromMethod(methodDesc protoreflect.MethodDescriptor) string {
	options := methodDesc.Options().(proto.Message)
	if options == nil {
		return ""
	}

	httpRule, ok := proto.GetExtension(options, annotations.E_Http).(*annotations.HttpRule)
	if !ok || httpRule == nil {
		return ""
	}

	switch p := httpRule.Pattern.(type) {
	case *annotations.HttpRule_Get:
		return p.Get
	case *annotations.HttpRule_Post:
		return p.Post
	case *annotations.HttpRule_Patch:
		return p.Patch
	case *annotations.HttpRule_Put:
		return p.Put
	case *annotations.HttpRule_Delete:
		return p.Delete
	default:
		return ""
	}
}
