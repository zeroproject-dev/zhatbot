package domain

type Permission string

const (
	PermAdmin     Permission = "admin"
	PermModerator Permission = "moderator"
	PermVip       Permission = "vip"
	PermUser      Permission = "user"
)

type Role struct {
	Name        string
	Permissions []Permission
	Priority    int
}
