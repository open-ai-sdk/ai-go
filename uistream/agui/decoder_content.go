package agui

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/open-ai-sdk/ai-go/aikit"
)

func userContent(raw json.RawMessage) ([]aikit.ContentPart, error) {
	if isAbsent(raw) {
		return nil, errors.New("agui: user content is required")
	}
	if text, err := optionalStringContent(raw); err == nil {
		return []aikit.ContentPart{aikit.TextPart(text)}, nil
	}
	var content []aguiInputContent
	if err := json.Unmarshal(raw, &content); err != nil {
		return nil, errors.New("agui: user content must be a string or InputContent array")
	}
	parts := make([]aikit.ContentPart, 0, len(content))
	for _, item := range content {
		part, err := convertInputContent(item)
		if err != nil {
			return nil, err
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return nil, errors.New("agui: user content must not be empty")
	}
	return parts, nil
}

func convertInputContent(content aguiInputContent) (aikit.ContentPart, error) {
	if content.Type == "text" {
		return aikit.TextPart(content.Text), nil
	}
	if content.Source.Value == "" {
		return aikit.ContentPart{}, fmt.Errorf("agui: %s content requires a source", content.Type)
	}
	switch content.Source.Type {
	case "url":
		return urlContent(content)
	case "data":
		return dataContent(content)
	}
	return aikit.ContentPart{},
		fmt.Errorf("agui: unsupported %s source type %q", content.Type, content.Source.Type)
}

func urlContent(content aguiInputContent) (aikit.ContentPart, error) {
	switch content.Type {
	case "image":
		return aikit.ImageURLPart(content.Source.Value), nil
	case "audio":
		return aikit.AudioURLPart(content.Source.Value, content.Source.MimeType), nil
	case "video":
		return aikit.VideoURLPart(content.Source.Value, content.Source.MimeType), nil
	case "document":
		return aikit.DocumentURLPart(content.Source.Value, content.Source.MimeType), nil
	}
	return aikit.ContentPart{}, fmt.Errorf("agui: unsupported url content type %q", content.Type)
}

func dataContent(content aguiInputContent) (aikit.ContentPart, error) {
	data, err := base64.StdEncoding.DecodeString(content.Source.Value)
	if err != nil {
		return aikit.ContentPart{}, fmt.Errorf("agui: decode %s content: %w", content.Type, err)
	}
	switch content.Type {
	case "image":
		return aikit.ImageDataPart(data, content.Source.MimeType), nil
	case "audio":
		return aikit.AudioDataPart(data, content.Source.MimeType), nil
	case "video":
		return aikit.VideoDataPart(data, content.Source.MimeType), nil
	case "document":
		return aikit.DocumentDataPart(data, content.Source.MimeType), nil
	}
	return aikit.ContentPart{}, fmt.Errorf("agui: unsupported data content type %q", content.Type)
}
