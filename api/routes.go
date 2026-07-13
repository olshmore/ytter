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
	RouteCreateUser                     = pb.Ytter_CreateUser_FullMethodName
	RouteVerifyEmail                    = pb.Ytter_VerifyEmail_FullMethodName
	RouteLoginUser                      = pb.Ytter_LoginUser_FullMethodName
	RouteRefreshToken                   = pb.Ytter_RefreshToken_FullMethodName
	RouteListUsers                      = pb.Ytter_ListUsers_FullMethodName
	RouteUpdateUser                     = pb.Ytter_UpdateUser_FullMethodName
	RouteInitiateGoogleAuth             = pb.Ytter_InitiateGoogleAuth_FullMethodName
	RouteGoogleAuthCallback             = pb.Ytter_GoogleAuthCallback_FullMethodName
	RouteListHostLocations              = pb.Ytter_ListHostLocations_FullMethodName
	RouteCreateHostLocation             = pb.Ytter_CreateHostLocation_FullMethodName
	RouteGetHostLocation                = pb.Ytter_GetHostLocation_FullMethodName
	RouteGetHostLocationBySlug          = pb.Ytter_GetHostLocationBySlug_FullMethodName
	RouteUpdateHostLocation             = pb.Ytter_UpdateHostLocation_FullMethodName
	RouteUpdateLocationBranding         = pb.Ytter_UpdateLocationBranding_FullMethodName
	RouteCreateLocationBrandingLogoUpload = pb.Ytter_CreateLocationBrandingLogoUpload_FullMethodName
	RouteListHostLocationBookings       = pb.Ytter_ListHostLocationBookings_FullMethodName
	RouteListHostLocationServices       = pb.Ytter_ListHostLocationServices_FullMethodName
	RouteCreateHostLocationService      = pb.Ytter_CreateHostLocationService_FullMethodName
	RouteUpdateHostLocationService      = pb.Ytter_UpdateHostLocationService_FullMethodName
	RouteListHostLocationSlots          = pb.Ytter_ListHostLocationSlots_FullMethodName
	RouteCreateHostLocationSlot         = pb.Ytter_CreateHostLocationSlot_FullMethodName
	RouteUpdateHostLocationSlot         = pb.Ytter_UpdateHostLocationSlot_FullMethodName
	RouteHostApproveBooking             = pb.Ytter_HostApproveBooking_FullMethodName
	RouteHostRejectBooking              = pb.Ytter_HostRejectBooking_FullMethodName
	RouteHostCancelBooking              = pb.Ytter_HostCancelBooking_FullMethodName
	RouteHostSetBookingNoShow           = pb.Ytter_HostSetBookingNoShow_FullMethodName
	RouteGetHostSetupChecklist          = pb.Ytter_GetHostSetupChecklist_FullMethodName
	RouteGetMyBookingRebookContext      = pb.Ytter_GetMyBookingRebookContext_FullMethodName
	RouteListMyBookings                 = pb.Ytter_ListMyBookings_FullMethodName
	RouteCancelMyBooking                = pb.Ytter_CancelMyBooking_FullMethodName
	RouteGetHostBookingAnalyticsSummary = pb.Ytter_GetHostBookingAnalyticsSummary_FullMethodName
	RouteListPublicSlots                = pb.Ytter_ListPublicSlots_FullMethodName
	RouteGetPublicCalendarAvailability  = pb.Ytter_GetPublicCalendarAvailability_FullMethodName
	RouteListPublicLocations            = pb.Ytter_ListPublicLocations_FullMethodName
	RouteGetPublicFilterOptions         = pb.Ytter_GetPublicFilterOptions_FullMethodName
	RouteCreatePublicBooking            = pb.Ytter_CreatePublicBooking_FullMethodName
	RouteCancelPublicBooking            = pb.Ytter_CancelPublicBooking_FullMethodName
	RouteJoinPublicWaitlist             = pb.Ytter_JoinPublicWaitlist_FullMethodName
	RoutePublicBookingAssistantSuggest  = pb.Ytter_PublicBookingAssistantSuggest_FullMethodName
	RouteCreateHostLocationSlotsBatch   = pb.Ytter_CreateHostLocationSlotsBatch_FullMethodName
	RouteHostSlotAssistantPreview       = pb.Ytter_HostSlotAssistantPreview_FullMethodName
	RouteHostSlotAssistantPublish       = pb.Ytter_HostSlotAssistantPublish_FullMethodName
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
		RouteCreateUser,                    // registration
		RouteLoginUser,                     // login
		RouteVerifyEmail,                   // email verification
		RouteRefreshToken,                  // token refresh
		RouteInitiateGoogleAuth,            // Google OAuth initiation
		RouteGoogleAuthCallback,            // Google OAuth callback
		RouteListPublicSlots,               // public slot discovery
		RouteGetPublicCalendarAvailability, // public calendar availability
		RouteListPublicLocations,           // public location discovery
		RouteGetPublicFilterOptions,        // public filter options
		RouteCreatePublicBooking,           // guest booking creation
		RouteCancelPublicBooking,           // guest booking cancellation
		RouteJoinPublicWaitlist,            // guest waitlist join
		RoutePublicBookingAssistantSuggest, // guest NL booking assistant
	}

	// ============================================================================
	// AUTHENTICATED: ANY ROLE
	// Routes accessible by any authenticated user
	// ============================================================================
	config.RequireAuth([]utils.Role{},
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
	// AUTHENTICATED: HOST OR ADMIN (booking management)
	// ============================================================================
	config.RequireAuth([]utils.Role{utils.RoleHost, utils.RoleAdmin},
		RouteListHostLocations,
		RouteCreateHostLocation,
		RouteGetHostLocation,
		RouteGetHostLocationBySlug,
		RouteUpdateHostLocation,
		RouteUpdateLocationBranding,
		RouteCreateLocationBrandingLogoUpload,
		RouteListHostLocationBookings,
		RouteListHostLocationServices,
		RouteCreateHostLocationService,
		RouteUpdateHostLocationService,
		RouteListHostLocationSlots,
		RouteCreateHostLocationSlot,
		RouteUpdateHostLocationSlot,
		RouteHostApproveBooking,
		RouteHostRejectBooking,
		RouteHostCancelBooking,
		RouteHostSetBookingNoShow,
		RouteGetHostSetupChecklist,
		RouteGetHostBookingAnalyticsSummary,
		RouteCreateHostLocationSlotsBatch,
		RouteHostSlotAssistantPreview,
		RouteHostSlotAssistantPublish,
	)

	// ============================================================================
	// AUTHENTICATED: CLIENT OR ADMIN
	// ============================================================================
	config.RequireAuth([]utils.Role{utils.RoleClient, utils.RoleAdmin},
		RouteGetMyBookingRebookContext,
		RouteListMyBookings,
		RouteCancelMyBooking,
	)

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
