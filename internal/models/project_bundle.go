package models

// ProjectBundle is a JSON snapshot of story data. Media files and execution
// history are intentionally not included; file references require the same host.
type ProjectBundle struct {
	Project    Project     `json:"project"`
	Characters []Character `json:"characters"`
	Scenes     []Scene     `json:"scenes"`
	Shots      []Shot      `json:"shots"`
	Scripts    []Script    `json:"scripts"`
	Assets     []Asset     `json:"assets"`
	Style      *StyleGuide `json:"style"`
}
