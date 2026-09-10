package service

import (
	"errors"
	"testing"

	"github.com/imrui/xray-pilot/config"
	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/entity"
)

func seedAdmins(t *testing.T, admins ...config.AdminUser) *AdminService {
	t.Helper()
	config.Global.Admins = admins
	if err := config.HashAdminPasswords(); err != nil {
		t.Fatalf("hash: %v", err)
	}
	svc := NewAdminService()
	if err := svc.SeedFromConfig(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return svc
}

// TestAdminSeedFromConfig 验证首次种子：第一个为 super_admin，其余 admin；表非空后再次调用不重复写入
func TestAdminSeedFromConfig(t *testing.T) {
	setupServiceTestDB(t)
	svc := seedAdmins(t,
		config.AdminUser{Username: "root", Password: "rootpass"},
		config.AdminUser{Username: "ops", Password: "opspass"},
	)

	list, err := svc.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 || list[0].Role != entity.RoleSuperAdmin || list[1].Role != entity.RoleAdmin {
		t.Fatalf("种子角色错误: %+v", list)
	}

	// 表非空：config 里换成别的账号也不再种入
	config.Global.Admins = []config.AdminUser{{Username: "intruder", PasswordHash: "x"}}
	if err := svc.SeedFromConfig(); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	list, _ = svc.List()
	if len(list) != 2 {
		t.Fatalf("表非空时不应再种入，got %d", len(list))
	}

	// 种子密码可登录
	if _, err := NewAuthService().Login(&dto.LoginRequest{Username: "root", Password: "rootpass"}); err != nil {
		t.Fatalf("种子账号登录失败: %v", err)
	}
}

// TestAdminLastSuperAdminGuard 验证最后一个超级管理员不能被删除 / 降级 / 禁用
func TestAdminLastSuperAdminGuard(t *testing.T) {
	setupServiceTestDB(t)
	svc := seedAdmins(t, config.AdminUser{Username: "root", Password: "rootpass"})
	list, _ := svc.List()
	rootID := list[0].ID

	other, err := svc.Create(&dto.CreateAdminRequest{Username: "ops", Password: "opspass"}, "admin:root")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	role := entity.RoleAdmin
	if _, err := svc.Update(rootID, &dto.UpdateAdminRequest{Role: &role}, other.ID, "admin:ops"); !errors.Is(err, ErrLastSuperAdmin) {
		t.Errorf("降级最后一个 super_admin 应被拒绝，got %v", err)
	}
	inactive := false
	if _, err := svc.Update(rootID, &dto.UpdateAdminRequest{Active: &inactive}, other.ID, "admin:ops"); !errors.Is(err, ErrLastSuperAdmin) {
		t.Errorf("禁用最后一个 super_admin 应被拒绝，got %v", err)
	}
	if err := svc.Delete(rootID, other.ID, "admin:ops"); !errors.Is(err, ErrLastSuperAdmin) {
		t.Errorf("删除最后一个 super_admin 应被拒绝，got %v", err)
	}
	if err := svc.Delete(rootID, rootID, "admin:root"); err == nil {
		t.Error("删除自己应被拒绝")
	}
	if _, err := svc.Update(rootID, &dto.UpdateAdminRequest{Role: &role}, rootID, "admin:root"); err == nil || errors.Is(err, ErrLastSuperAdmin) {
		t.Errorf("改自己的角色应以「不能修改自己」拒绝，而非最后 super_admin 护栏，got %v", err)
	}
	if _, err := svc.Update(rootID, &dto.UpdateAdminRequest{Active: &inactive}, rootID, "admin:root"); err == nil {
		t.Error("禁用自己应被拒绝")
	}

	// 提升 ops 为 super_admin 后，root 即可降级
	superRole := entity.RoleSuperAdmin
	if _, err := svc.Update(other.ID, &dto.UpdateAdminRequest{Role: &superRole}, rootID, "admin:root"); err != nil {
		t.Fatalf("提升 ops: %v", err)
	}
	if _, err := svc.Update(rootID, &dto.UpdateAdminRequest{Role: &role}, other.ID, "admin:ops"); err != nil {
		t.Errorf("有第二个 super_admin 时降级应成功，got %v", err)
	}
}

// TestAdminPasswordFlows 验证改自己密码需旧密码、重置密码不需旧密码、禁用账号不能登录
func TestAdminPasswordFlows(t *testing.T) {
	setupServiceTestDB(t)
	svc := seedAdmins(t, config.AdminUser{Username: "root", Password: "rootpass"})
	list, _ := svc.List()
	rootID := list[0].ID
	auth := NewAuthService()

	if err := svc.ChangeOwnPassword(rootID, &dto.ChangePasswordRequest{OldPassword: "wrong", NewPassword: "newpass1"}, "admin:root"); err == nil {
		t.Error("旧密码错误应被拒绝")
	}
	if err := svc.ChangeOwnPassword(rootID, &dto.ChangePasswordRequest{OldPassword: "rootpass", NewPassword: "short"}, "admin:root"); err == nil {
		t.Error("过短密码应被拒绝")
	}
	if err := svc.ChangeOwnPassword(rootID, &dto.ChangePasswordRequest{OldPassword: "rootpass", NewPassword: "newpass1"}, "admin:root"); err != nil {
		t.Fatalf("改密: %v", err)
	}
	if _, err := auth.Login(&dto.LoginRequest{Username: "root", Password: "newpass1"}); err != nil {
		t.Fatalf("新密码登录失败: %v", err)
	}

	ops, err := svc.Create(&dto.CreateAdminRequest{Username: "ops", Password: "opspass"}, "admin:root")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.SetPassword(rootID, "resetpw1", rootID, "admin:root"); err == nil {
		t.Error("重置自己的密码应被拒绝，只能走 ChangeOwnPassword")
	}
	if err := svc.SetPassword(ops.ID, "resetpw1", rootID, "admin:root"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	resp, err := auth.Login(&dto.LoginRequest{Username: "ops", Password: "resetpw1"})
	if err != nil {
		t.Fatalf("重置后登录失败: %v", err)
	}
	if resp.Role != entity.RoleAdmin || resp.Username != "ops" {
		t.Errorf("登录响应角色/用户名错误: %+v", resp)
	}

	inactive := false
	if _, err := svc.Update(ops.ID, &dto.UpdateAdminRequest{Active: &inactive}, rootID, "admin:root"); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if _, err := auth.Login(&dto.LoginRequest{Username: "ops", Password: "resetpw1"}); err == nil {
		t.Error("禁用账号不应能登录")
	}
}
