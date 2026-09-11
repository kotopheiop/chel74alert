package models

import "time"

type Kind string

const (
	KindDanger Kind = "danger"
	KindClear  Kind = "clear"
)

type Alert struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary"`
	URL       string    `json:"url"`
	Source    string    `json:"source"`
	Published time.Time `json:"published"`
	Kind      Kind      `json:"kind"`
}
