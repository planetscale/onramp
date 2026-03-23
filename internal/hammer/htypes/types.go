// Package htypes
package htypes

import "time"

// UserConfig holds configuration for user simulation.
type UserConfig struct {
	TotalUsers            int           `yaml:"total-users"`
	NewUsersPerSecond     float64       `yaml:"new_users-per-second"`
	DeletedUsersPerSecond float64       `yaml:"deleted-users-per-second"`
	UserFlowDelay         time.Duration `yaml:"user-flow-delay"`
	MaxUsers              int           `yaml:"max-users"`
	MinUsers              int           `yaml:"min-users"`
	Flows                 []Flow        `yaml:"flows"`
}

// Flow represents a named set of weighted SQL queries.
type Flow struct {
	Name    string   `yaml:"name"`
	Weight  float64  `yaml:"weight"`
	Queries []string `yaml:"queries"`
}
