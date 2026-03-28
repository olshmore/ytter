package utils

type Role string

const (
	RoleAdmin  Role = "admin"
	RoleHost   Role = "host"
	RoleClient Role = "client"
)

func (r Role) String() string {
	return string(r)
}

func IsCanonicalRole(role Role) bool {
	return role == RoleAdmin || role == RoleHost || role == RoleClient
}

func RolesFromStrings(roles []string) []Role {
	if len(roles) == 0 {
		return []Role{RoleClient}
	}

	out := make([]Role, 0, len(roles))
	for _, role := range roles {
		out = append(out, Role(role))
	}
	return out
}
