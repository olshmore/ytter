package access

import (
	"testing"
	"time"

	"github.com/olshmore/ytter/pkg/token"
	"github.com/olshmore/ytter/pkg/utils"
	"github.com/stretchr/testify/require"
)

func TestHostMayAccessLocation(t *testing.T) {
	t.Parallel()
	owner := "biz_owner"

	admin, err := token.NewPayload("admin_user", []utils.Role{utils.RoleAdmin}, time.Hour)
	require.NoError(t, err)
	hostOk, err := token.NewPayload(owner, []utils.Role{utils.RoleHost}, time.Hour)
	require.NoError(t, err)
	hostOther, err := token.NewPayload("other", []utils.Role{utils.RoleHost}, time.Hour)
	require.NoError(t, err)
	client, err := token.NewPayload("c1", []utils.Role{utils.RoleClient}, time.Hour)
	require.NoError(t, err)

	require.True(t, HostMayAccessLocation(admin, owner))
	require.True(t, HostMayAccessLocation(hostOk, owner))
	require.False(t, HostMayAccessLocation(hostOther, owner))
	require.False(t, HostMayAccessLocation(client, owner))
	require.False(t, HostMayAccessLocation(nil, owner))
}

func TestClientMayAccessLinkedBooking(t *testing.T) {
	t.Parallel()
	u := "client_a"
	link := u
	admin, err := token.NewPayload("adm", []utils.Role{utils.RoleAdmin}, time.Hour)
	require.NoError(t, err)
	client, err := token.NewPayload(u, []utils.Role{utils.RoleClient}, time.Hour)
	require.NoError(t, err)
	host, err := token.NewPayload(u, []utils.Role{utils.RoleHost}, time.Hour)
	require.NoError(t, err)

	require.True(t, ClientMayAccessLinkedBooking(admin, &link))
	require.True(t, ClientMayAccessLinkedBooking(client, &link))
	require.False(t, ClientMayAccessLinkedBooking(client, ptr("other")))
	require.False(t, ClientMayAccessLinkedBooking(client, nil))
	require.False(t, ClientMayAccessLinkedBooking(host, &link))
	require.False(t, ClientMayAccessLinkedBooking(nil, &link))
}

func TestGuestEmailMatchesUser(t *testing.T) {
	t.Parallel()
	require.True(t, GuestEmailMatchesUser("A@B.C", "a@b.c"))
	require.False(t, GuestEmailMatchesUser("a@b.c", "x@y.z"))
}

func ptr(s string) *string { return &s }
