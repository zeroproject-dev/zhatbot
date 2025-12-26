package domain

import "time"

type NotificationType string

const (
	NotificationSubscription   NotificationType = "subscription"
	NotificationDonation       NotificationType = "donation"
	NotificationBits           NotificationType = "bits"
	NotificationGiveawayWinner NotificationType = "giveaway_winner"
	NotificationGeneric        NotificationType = "generic"
)

type Notification struct {
	ID        int64
	Type      NotificationType
	Platform  Platform
	Username  string
	Amount    float64
	Message   string
	Metadata  map[string]string
	CreatedAt time.Time
}
