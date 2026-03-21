package telegram

type UserRole int

const (
	RoleObserver UserRole = iota
	RoleOperator
	RoleAdmin
)

type Authorizer struct {
	users map[int64]UserRole
}

func NewAuthorizer(adminIDs, operatorIDs []int64) *Authorizer {
	users := make(map[int64]UserRole)
	for _, id := range operatorIDs {
		users[id] = RoleOperator
	}
	for _, id := range adminIDs {
		users[id] = RoleAdmin
	}
	return &Authorizer{users: users}
}

func (a *Authorizer) RoleOf(userID int64) UserRole {
	if role, ok := a.users[userID]; ok {
		return role
	}
	return RoleObserver
}

func (a *Authorizer) CanSendCommand(userID int64) bool {
	role := a.RoleOf(userID)
	return role == RoleOperator || role == RoleAdmin
}

func (a *Authorizer) CanApproveElevation(userID int64) bool {
	return a.RoleOf(userID) == RoleAdmin
}
