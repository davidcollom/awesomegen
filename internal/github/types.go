package github

import "time"

type RepoMeta struct {
	FullName    string
	URL         string
	Description string
	Stars       int
	License     string
	Topics      []string
	Archived    bool
	PushedAt    time.Time
}
