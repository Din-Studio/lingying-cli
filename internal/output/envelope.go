// Copyright (c) 2026 Lingying. SPDX-License-Identifier: MIT
// Unified output envelope.
// Every command emits through this — Agents only check "ok".
package output

import (
	"encoding/json"
	"fmt"
)

type Envelope struct {
	OK   bool  `json:"ok"`
	Data any   `json:"data,omitempty"`
	Meta *Meta `json:"meta,omitempty"`
}

type Meta struct {
	Cost     string `json:"cost,omitempty"`
	Duration int64  `json:"duration,omitempty"`
	TaskID   string `json:"task_id,omitempty"`
	Count    int    `json:"count,omitempty"`
}

// Failure builds the stable error payload consumed by non-interactive callers.
// Extra fields (for example task_id) may be merged into data by the caller.
func Failure(code, message string, extra map[string]any) Envelope {
	data := map[string]any{"code": code, "message": message}
	for key, value := range extra {
		data[key] = value
	}
	return Envelope{OK: false, Data: data}
}

func JSON(env Envelope) error {
	if !env.OK {
		if data, ok := env.Data.(map[string]any); ok {
			if _, exists := data["code"]; !exists {
				data["code"] = "command_error"
			}
		}
	}
	b, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}
