package repository

import (
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
)

type AdminRepository struct{}

func NewAdminRepository() *AdminRepository {
	return &AdminRepository{}
}

func (r *AdminRepository) Count() (int64, error) {
	var total int64
	return total, DB.Model(&entity.AdminUser{}).Count(&total).Error
}

func (r *AdminRepository) Create(admin *entity.AdminUser) error {
	return DB.Create(admin).Error
}

func (r *AdminRepository) FindByID(id uint) (*entity.AdminUser, error) {
	var admin entity.AdminUser
	err := DB.First(&admin, id).Error
	return &admin, err
}

func (r *AdminRepository) FindByUsername(username string) (*entity.AdminUser, error) {
	var admin entity.AdminUser
	err := DB.Where("username = ?", username).First(&admin).Error
	return &admin, err
}

func (r *AdminRepository) List() ([]entity.AdminUser, error) {
	var admins []entity.AdminUser
	err := DB.Order("id asc").Find(&admins).Error
	return admins, err
}

// Update 全量保存（Role / Active 等字段变更）
func (r *AdminRepository) Update(admin *entity.AdminUser) error {
	return DB.Save(admin).Error
}

func (r *AdminRepository) UpdateActive(id uint, active bool) error {
	return DB.Model(&entity.AdminUser{}).Where("id = ?", id).Update("active", active).Error
}

func (r *AdminRepository) UpdatePasswordHash(id uint, hash string) error {
	return DB.Model(&entity.AdminUser{}).Where("id = ?", id).Update("password_hash", hash).Error
}

func (r *AdminRepository) UpdateLastLogin(id uint, at time.Time) error {
	return DB.Model(&entity.AdminUser{}).Where("id = ?", id).Update("last_login_at", at).Error
}

func (r *AdminRepository) Delete(id uint) error {
	return DB.Delete(&entity.AdminUser{}, id).Error
}

// CountActiveSuperAdminsExcluding 统计启用态超级管理员数量（排除指定 id），用于「最后一个 super_admin」护栏
func (r *AdminRepository) CountActiveSuperAdminsExcluding(excludeID uint) (int64, error) {
	var total int64
	err := DB.Model(&entity.AdminUser{}).
		Where("role = ? AND active = ? AND id <> ?", entity.RoleSuperAdmin, true, excludeID).
		Count(&total).Error
	return total, err
}
