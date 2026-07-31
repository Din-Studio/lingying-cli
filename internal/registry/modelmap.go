// Copyright (c) 2026 Lingying. SPDX-License-Identifier: MIT
// Model-type → endpoint registry.
// 6 entries cover all current model_types. Add one line for a new type.
package registry

// Entry describes how to call a model_type.
type Entry struct {
	Mode         string // "sync" | "async"
	Endpoint     string // submit POST endpoint
	OutputType   string // "text" | "image" | "video" | "audio"
	OutputExt    string // "png" | "mp4" | "mp3" | "txt"
}

// modelTypeMap defines the 6 model_type → endpoint rules.
// Add one entry when Gateway adds a new model_type.
var modelTypeMap = map[string]Entry{
	"text":        {Mode: "sync", Endpoint: "/v1/chat/completions", OutputType: "text", OutputExt: "txt"},
	"image":       {Mode: "async", Endpoint: "/v1/images/generations", OutputType: "image", OutputExt: "png"},
	"image_edit":  {Mode: "async", Endpoint: "/v1/images/generations", OutputType: "image", OutputExt: "png"},
	"video":       {Mode: "async", Endpoint: "/v1/videos/generations", OutputType: "video", OutputExt: "mp4"},
	"video_edit":  {Mode: "async", Endpoint: "/v1/videos/generations", OutputType: "video", OutputExt: "mp4"},
	"audio_edit":  {Mode: "async", Endpoint: "/v1/audios/generations", OutputType: "audio", OutputExt: "mp3"},
}

func Lookup(modelType string) (Entry, bool) {
	e, ok := modelTypeMap[modelType]
	return e, ok
}

func AllTypes() []string {
	types := make([]string, 0, len(modelTypeMap))
	for t := range modelTypeMap {
		types = append(types, t)
	}
	return types
}
