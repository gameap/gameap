package compatrust

import (
	"context"
	"sync"

	"github.com/gameap/gameap/pkg/plugin/sdk/authz"
	"github.com/gameap/gameap/pkg/plugin/sdk/net"
	"github.com/gameap/gameap/pkg/plugin/sdk/rbac"
	"github.com/gameap/gameap/pkg/plugin/sdk/scheduler"
)

// This file stubs the host services of the SDK modules added in panel 4.4
// (authz, rbac, scheduler, net). Those packages do not exist on v4.3.5, so CI
// does not copy this file there; it must compile on v4.4.1 and master only.

// stubAuthzService satisfies authz.AuthzService; every ability check is allowed.
type stubAuthzService struct {
	callRecorder
}

func (s *stubAuthzService) Can(_ context.Context, _ *authz.CanRequest) (*authz.CanResponse, error) {
	s.record("Can")

	return &authz.CanResponse{Allowed: true}, nil
}

func (s *stubAuthzService) CanOneOf(_ context.Context, _ *authz.CanRequest) (*authz.CanResponse, error) {
	s.record("CanOneOf")

	return &authz.CanResponse{Allowed: true}, nil
}

func (s *stubAuthzService) CanForEntity(
	_ context.Context,
	_ *authz.CanForEntityRequest,
) (*authz.CanResponse, error) {
	s.record("CanForEntity")

	return &authz.CanResponse{Allowed: true}, nil
}

func (s *stubAuthzService) CanAnyForEntity(
	_ context.Context,
	_ *authz.CanForEntityRequest,
) (*authz.CanResponse, error) {
	s.record("CanAnyForEntity")

	return &authz.CanResponse{Allowed: true}, nil
}

func (s *stubAuthzService) GetUserRoles(
	_ context.Context,
	_ *authz.GetUserRolesRequest,
) (*authz.GetUserRolesResponse, error) {
	s.record("GetUserRoles")

	return &authz.GetUserRolesResponse{Roles: []string{"admin"}}, nil
}

// stubRBACService satisfies rbac.RBACService; every mutation succeeds.
type stubRBACService struct {
	callRecorder
}

func (s *stubRBACService) SetUserRoles(
	_ context.Context,
	_ *rbac.SetUserRolesRequest,
) (*rbac.Result, error) {
	s.record("SetUserRoles")

	return &rbac.Result{Success: true}, nil
}

func (s *stubRBACService) AllowUserAbilitiesForEntity(
	_ context.Context,
	_ *rbac.UserAbilitiesRequest,
) (*rbac.Result, error) {
	s.record("AllowUserAbilitiesForEntity")

	return &rbac.Result{Success: true}, nil
}

func (s *stubRBACService) RevokeOrForbidUserAbilitiesForEntity(
	_ context.Context,
	_ *rbac.UserAbilitiesRequest,
) (*rbac.Result, error) {
	s.record("RevokeOrForbidUserAbilitiesForEntity")

	return &rbac.Result{Success: true}, nil
}

func (s *stubRBACService) GetRoles(
	_ context.Context,
	_ *rbac.GetRolesRequest,
) (*rbac.GetRolesResponse, error) {
	s.record("GetRoles")

	return &rbac.GetRolesResponse{Roles: []*rbac.Role{{Id: 1, Name: "admin"}}}, nil
}

func (s *stubRBACService) SaveRole(
	_ context.Context,
	req *rbac.SaveRoleRequest,
) (*rbac.SaveRoleResponse, error) {
	s.record("SaveRole")

	return &rbac.SaveRoleResponse{Success: true, Role: req.Role}, nil
}

func (s *stubRBACService) DeleteRole(
	_ context.Context,
	_ *rbac.DeleteRoleRequest,
) (*rbac.Result, error) {
	s.record("DeleteRole")

	return &rbac.Result{Success: true}, nil
}

func (s *stubRBACService) GetPermissions(
	_ context.Context,
	_ *rbac.EntityRequest,
) (*rbac.GetPermissionsResponse, error) {
	s.record("GetPermissions")

	return &rbac.GetPermissionsResponse{
		Permissions: []*rbac.Permission{{Id: 1, Ability: &rbac.Ability{Name: "game-server-start"}}},
	}, nil
}

func (s *stubRBACService) GetRolesForEntity(
	_ context.Context,
	_ *rbac.EntityRequest,
) (*rbac.GetRolesForEntityResponse, error) {
	s.record("GetRolesForEntity")

	return &rbac.GetRolesForEntityResponse{
		Roles: []*rbac.RestrictedRole{{Role: &rbac.Role{Id: 1, Name: "admin"}}},
	}, nil
}

func (s *stubRBACService) AssignRolesForEntity(
	_ context.Context,
	_ *rbac.AssignRolesRequest,
) (*rbac.Result, error) {
	s.record("AssignRolesForEntity")

	return &rbac.Result{Success: true}, nil
}

func (s *stubRBACService) ClearRolesForEntity(
	_ context.Context,
	_ *rbac.EntityRequest,
) (*rbac.Result, error) {
	s.record("ClearRolesForEntity")

	return &rbac.Result{Success: true}, nil
}

func (s *stubRBACService) Allow(_ context.Context, _ *rbac.AbilitiesRequest) (*rbac.Result, error) {
	s.record("Allow")

	return &rbac.Result{Success: true}, nil
}

func (s *stubRBACService) Forbid(_ context.Context, _ *rbac.AbilitiesRequest) (*rbac.Result, error) {
	s.record("Forbid")

	return &rbac.Result{Success: true}, nil
}

func (s *stubRBACService) Revoke(_ context.Context, _ *rbac.AbilitiesRequest) (*rbac.Result, error) {
	s.record("Revoke")

	return &rbac.Result{Success: true}, nil
}

// stubSchedulerService satisfies scheduler.SchedulerService and keeps the
// registered tasks in memory.
type stubSchedulerService struct {
	callRecorder
	mu    sync.Mutex
	tasks map[string]*scheduler.TaskInfo
}

func (s *stubSchedulerService) AddTask(
	_ context.Context,
	req *scheduler.AddTaskRequest,
) (*scheduler.AddTaskResponse, error) {
	s.record("AddTask")
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.tasks == nil {
		s.tasks = make(map[string]*scheduler.TaskInfo)
	}
	s.tasks[req.Name] = &scheduler.TaskInfo{
		Name:        req.Name,
		IntervalMs:  req.IntervalMs,
		ErrorPolicy: req.ErrorPolicy,
		TimeoutMs:   req.TimeoutMs,
	}

	return &scheduler.AddTaskResponse{Success: true}, nil
}

func (s *stubSchedulerService) RemoveTask(
	_ context.Context,
	req *scheduler.RemoveTaskRequest,
) (*scheduler.RemoveTaskResponse, error) {
	s.record("RemoveTask")
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.tasks, req.Name)

	return &scheduler.RemoveTaskResponse{Success: true}, nil
}

func (s *stubSchedulerService) ListTasks(
	_ context.Context,
	_ *scheduler.ListTasksRequest,
) (*scheduler.ListTasksResponse, error) {
	s.record("ListTasks")
	s.mu.Lock()
	defer s.mu.Unlock()

	tasks := make([]*scheduler.TaskInfo, 0, len(s.tasks))
	for _, task := range s.tasks {
		tasks = append(tasks, task)
	}

	return &scheduler.ListTasksResponse{Tasks: tasks}, nil
}

// TaskNames returns the names of the currently registered tasks.
func (s *stubSchedulerService) TaskNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	names := make([]string, 0, len(s.tasks))
	for name := range s.tasks {
		names = append(names, name)
	}

	return names
}

// stubNetService satisfies net.NetService with in-memory responses.
type stubNetService struct {
	callRecorder
}

func (s *stubNetService) Send(_ context.Context, req *net.NetSendRequest) (*net.NetSendResponse, error) {
	s.record("Send")

	return &net.NetSendResponse{Written: int32(len(req.Data))}, nil
}

func (s *stubNetService) Recv(_ context.Context, _ *net.NetRecvRequest) (*net.NetRecvResponse, error) {
	s.record("Recv")

	return &net.NetRecvResponse{Data: []byte("ok")}, nil
}

func (s *stubNetService) Close(_ context.Context, _ *net.NetCloseRequest) (*net.NetCloseResponse, error) {
	s.record("Close")

	return &net.NetCloseResponse{}, nil
}
