// Copyright (c) 2026 Lingying. SPDX-License-Identifier: MIT
// Unified output envelope.
// Every command emits through this — Agents only check "ok".
package output

import (
	"encoding/json"
	"fmt"
)

type Envelope struct {
	OK   bool   `json:"ok"`
	Data any    `json:"data,omitempty"`
	Meta *Meta  `json:"meta,omitempty"`
}

type Meta struct {
	Cost     string `json:"cost,omitempty"`
	Duration int64  `json:"duration,omitempty"`
	TaskID   string `json:"task_id,omitempty"`
	Count    int    `json:"count,omitempty"`
}

func JSON(env Envelope) error {
	b, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}
