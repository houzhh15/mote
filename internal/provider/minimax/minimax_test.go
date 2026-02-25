package minimax

import (
	"encoding/json"
	"testing"
)

func TestContextWindow_WithPrefix(t *testing.T) {
	p := &MinimaxProvider{model: "MiniMax-M2.5"}

	// Session model stored with "minimax:" prefix
	cw := p.ContextWindow("minimax:MiniMax-M2.5")
	if cw != 204800 {
		t.Errorf("ContextWindow(\"minimax:MiniMax-M2.5\") = %d, want 204800", cw)
	}
}

func TestContextWindow_WithoutPrefix(t *testing.T) {
	p := &MinimaxProvider{model: "MiniMax-M2.5"}

	cw := p.ContextWindow("MiniMax-M2.5")
	if cw != 204800 {
		t.Errorf("ContextWindow(\"MiniMax-M2.5\") = %d, want 204800", cw)
	}
}

func TestContextWindow_Empty_FallsBackToProviderModel(t *testing.T) {
	p := &MinimaxProvider{model: "MiniMax-M2.5"}

	cw := p.ContextWindow("")
	if cw != 204800 {
		t.Errorf("ContextWindow(\"\") = %d, want 204800", cw)
	}
}

func TestContextWindow_AllModels(t *testing.T) {
	p := &MinimaxProvider{}
	for name, meta := range ModelMetadata {
		// without prefix
		if cw := p.ContextWindow(name); cw != meta.ContextWindow {
			t.Errorf("ContextWindow(%q) = %d, want %d", name, cw, meta.ContextWindow)
		}
		// with prefix
		prefixed := "minimax:" + name
		if cw := p.ContextWindow(prefixed); cw != meta.ContextWindow {
			t.Errorf("ContextWindow(%q) = %d, want %d", prefixed, cw, meta.ContextWindow)
		}
	}
}

func TestChatMessage_MarshalJSON_StringContent(t *testing.T) {
	msg := chatMessage{
		Role:    "user",
		Content: strPtr("Hello, world!"),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(data, &result)

	if result["role"] != "user" {
		t.Errorf("role = %v, want user", result["role"])
	}
	if result["content"] != "Hello, world!" {
		t.Errorf("content = %v, want string", result["content"])
	}
}

func TestChatMessage_MarshalJSON_NullContent(t *testing.T) {
	msg := chatMessage{
		Role:    "assistant",
		Content: nil, // null for assistant messages with only tool_calls
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(data, &result)

	if result["content"] != nil {
		t.Errorf("content = %v, want null", result["content"])
	}
}

func TestChatMessage_MarshalJSON_MultipartVision(t *testing.T) {
	msg := chatMessage{
		Role: "user",
		ContentParts: []contentPart{
			{Type: "text", Text: "What's in this image?"},
			{Type: "image_url", ImageURL: &visionImageURL{URL: "data:image/jpeg;base64,/9j/4AAQ..."}},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(data, &result)

	// Content should be an array
	contentArr, ok := result["content"].([]interface{})
	if !ok {
		t.Fatalf("content should be an array, got %T: %v", result["content"], result["content"])
	}

	if len(contentArr) != 2 {
		t.Fatalf("content array should have 2 elements, got %d", len(contentArr))
	}

	// First element: text
	textPart := contentArr[0].(map[string]interface{})
	if textPart["type"] != "text" {
		t.Errorf("content[0].type = %v, want text", textPart["type"])
	}
	if textPart["text"] != "What's in this image?" {
		t.Errorf("content[0].text = %v, want expected text", textPart["text"])
	}

	// Second element: image_url
	imgPart := contentArr[1].(map[string]interface{})
	if imgPart["type"] != "image_url" {
		t.Errorf("content[1].type = %v, want image_url", imgPart["type"])
	}
	imgURL := imgPart["image_url"].(map[string]interface{})
	if imgURL["url"] != "data:image/jpeg;base64,/9j/4AAQ..." {
		t.Errorf("content[1].image_url.url = %v, unexpected", imgURL["url"])
	}
}

func TestChatMessage_UnmarshalJSON(t *testing.T) {
	data := []byte(`{"role":"assistant","content":"Hello!","tool_calls":[]}`)

	var msg chatMessage
	err := json.Unmarshal(data, &msg)
	if err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}

	if msg.Role != "assistant" {
		t.Errorf("role = %v, want assistant", msg.Role)
	}
	if msg.Content == nil || *msg.Content != "Hello!" {
		t.Errorf("content = %v, want 'Hello!'", msg.Content)
	}
}

func TestChatMessage_ContentParts_OverridesContent(t *testing.T) {
	// When ContentParts is set, it should be used even if Content is also set
	msg := chatMessage{
		Role:    "user",
		Content: strPtr("ignored"),
		ContentParts: []contentPart{
			{Type: "text", Text: "used instead"},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(data, &result)

	contentArr, ok := result["content"].([]interface{})
	if !ok {
		t.Fatalf("content should be an array when ContentParts is set, got %T", result["content"])
	}
	textPart := contentArr[0].(map[string]interface{})
	if textPart["text"] != "used instead" {
		t.Errorf("text = %v, want 'used instead'", textPart["text"])
	}
}
