package store

import (
	"context"
	"encoding/json"
)

type Store interface {
	Save(ctx context.Context, id string, v *Process) error
	Get(ctx context.Context, id string) (*Process, error)
	Delete(ctx context.Context, id string) error
	Walk(ctx context.Context, fn func(string, *Process) error) error
}

type Process struct {
	Id        string   `json:"id"`
	Subsystem []string `json:"subsystem,omitempty"`
}

func (p *Process) Encode() ([]byte, error) {
	return json.Marshal(p)
}

func (p *Process) Decode(data []byte) error {
	return json.Unmarshal(data, p)
}
