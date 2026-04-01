package server

import "github.com/stockyard-dev/stockyard-telegraph/internal/license"

type Limits struct {
	MaxEvents        int  // 0 = unlimited
	MaxSubscriptions int  // 0 = unlimited
	MaxFiresMonth    int  // 0 = unlimited
	HMACSigning      bool // Pro
	RetryDeliveries  bool // Pro
	RetentionDays    int
}

var freeLimits = Limits{
	MaxEvents:        3,
	MaxSubscriptions: 5,
	MaxFiresMonth:    1000,
	HMACSigning:      false,
	RetryDeliveries:  false,
	RetentionDays:    7,
}

var proLimits = Limits{
	MaxEvents:        0,
	MaxSubscriptions: 0,
	MaxFiresMonth:    0,
	HMACSigning:      true,
	RetryDeliveries:  true,
	RetentionDays:    90,
}

func LimitsFor(info *license.Info) Limits {
	if info != nil && info.IsPro() {
		return proLimits
	}
	return freeLimits
}

func LimitReached(limit, current int) bool {
	if limit == 0 {
		return false
	}
	return current >= limit
}
