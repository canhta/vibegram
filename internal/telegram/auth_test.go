package telegram

import "testing"

func makeAuth() *Authorizer {
	return NewAuthorizer([]int64{1001}, []int64{1002})
}

func TestAuthAdminRole(t *testing.T) {
	a := makeAuth()
	if a.RoleOf(1001) != RoleAdmin {
		t.Errorf("expected RoleAdmin for admin user")
	}
}

func TestAuthOperatorRole(t *testing.T) {
	a := makeAuth()
	if a.RoleOf(1002) != RoleOperator {
		t.Errorf("expected RoleOperator for operator user")
	}
}

func TestAuthObserverRole(t *testing.T) {
	a := makeAuth()
	if a.RoleOf(1003) != RoleObserver {
		t.Errorf("expected RoleObserver for known observer user")
	}
}

func TestAuthUnknownUserIsObserver(t *testing.T) {
	a := makeAuth()
	if a.RoleOf(9999) != RoleObserver {
		t.Errorf("expected unknown user to be RoleObserver")
	}
}

func TestAuthCanSendCommandAdmin(t *testing.T) {
	a := makeAuth()
	if !a.CanSendCommand(1001) {
		t.Errorf("expected admin to be able to send commands")
	}
}

func TestAuthCanSendCommandOperator(t *testing.T) {
	a := makeAuth()
	if !a.CanSendCommand(1002) {
		t.Errorf("expected operator to be able to send commands")
	}
}

func TestAuthCanSendCommandObserver(t *testing.T) {
	a := makeAuth()
	if a.CanSendCommand(9999) {
		t.Errorf("expected observer to not be able to send commands")
	}
}

func TestAuthCanApproveElevationAdminOnly(t *testing.T) {
	a := makeAuth()
	if !a.CanApproveElevation(1001) {
		t.Errorf("expected admin to be able to approve elevation")
	}
	if a.CanApproveElevation(1002) {
		t.Errorf("expected operator to not be able to approve elevation")
	}
	if a.CanApproveElevation(9999) {
		t.Errorf("expected observer to not be able to approve elevation")
	}
}
