package hostapi

import (
	"context"
	"os"
	"testing"

	"github.com/dop251/goja"
	"github.com/rs/zerolog"
)

func TestRegister(t *testing.T) {
	vm := goja.New()
	hctx := &Context{
		Ctx:         context.Background(),
		Logger:      zerolog.Nop(),
		ScriptName:  "test.js",
		ExecutionID: "test-123",
		Config:      DefaultConfig(),
	}

	err := Register(vm, hctx)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Verify mote object exists
	mote := vm.Get("mote")
	if mote == nil || goja.IsUndefined(mote) {
		t.Fatal("mote object not found")
	}

	// Verify sub-objects exist
	moteObj := mote.ToObject(vm)

	http := moteObj.Get("http")
	if http == nil || goja.IsUndefined(http) {
		t.Error("mote.http not found")
	}

	kv := moteObj.Get("kv")
	if kv == nil || goja.IsUndefined(kv) {
		t.Error("mote.kv not found")
	}

	fs := moteObj.Get("fs")
	if fs == nil || goja.IsUndefined(fs) {
		t.Error("mote.fs not found")
	}

	log := moteObj.Get("log")
	if log == nil || goja.IsUndefined(log) {
		t.Error("mote.log not found")
	}

	// Verify console exists
	console := vm.Get("console")
	if console == nil || goja.IsUndefined(console) {
		t.Error("console not found")
	}
}

func TestUnregister(t *testing.T) {
	vm := goja.New()
	hctx := &Context{
		Ctx:         context.Background(),
		Logger:      zerolog.Nop(),
		ScriptName:  "test.js",
		ExecutionID: "test-123",
		Config:      DefaultConfig(),
	}

	err := Register(vm, hctx)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	Unregister(vm)

	mote := vm.Get("mote")
	if mote != nil && !goja.IsUndefined(mote) {
		t.Error("mote should be undefined after Unregister")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if len(cfg.AllowedPaths) != 2 {
		t.Errorf("expected 2 allowed paths, got %d", len(cfg.AllowedPaths))
	}

	if cfg.MaxWriteSize != 10*1024*1024 {
		t.Errorf("expected MaxWriteSize 10MB, got %d", cfg.MaxWriteSize)
	}
}

func TestFSRead(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := tmpDir + "/test.txt"
	if err := os.WriteFile(tmpFile, []byte("hello world"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	vm := goja.New()
	hctx := &Context{
		Ctx:         context.Background(),
		Logger:      zerolog.Nop(),
		ScriptName:  "test.js",
		ExecutionID: "test-123",
		Config: Config{
			AllowedPaths: []string{tmpDir},
			MaxWriteSize: 10 * 1024 * 1024,
		},
	}

	err := Register(vm, hctx)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	vm.Set("testPath", tmpFile)
	result, err := vm.RunString(`mote.fs.read(testPath)`)
	if err != nil {
		t.Fatalf("fs.read failed: %v", err)
	}

	if result.String() != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", result.String())
	}
}

func TestFSWrite(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := tmpDir + "/output.txt"

	vm := goja.New()
	hctx := &Context{
		Ctx:         context.Background(),
		Logger:      zerolog.Nop(),
		ScriptName:  "test.js",
		ExecutionID: "test-123",
		Config: Config{
			AllowedPaths: []string{tmpDir},
			MaxWriteSize: 10 * 1024 * 1024,
		},
	}

	err := Register(vm, hctx)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	vm.Set("testPath", tmpFile)
	_, err = vm.RunString(`mote.fs.write(testPath, "test content")`)
	if err != nil {
		t.Fatalf("fs.write failed: %v", err)
	}

	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}

	if string(content) != "test content" {
		t.Errorf("expected 'test content', got '%s'", string(content))
	}
}

func TestFSExists(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := tmpDir + "/exists.txt"
	if err := os.WriteFile(tmpFile, []byte(""), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	vm := goja.New()
	hctx := &Context{
		Ctx:         context.Background(),
		Logger:      zerolog.Nop(),
		ScriptName:  "test.js",
		ExecutionID: "test-123",
		Config: Config{
			AllowedPaths: []string{tmpDir},
			MaxWriteSize: 10 * 1024 * 1024,
		},
	}

	err := Register(vm, hctx)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	vm.Set("existsPath", tmpFile)
	result, err := vm.RunString(`mote.fs.exists(existsPath)`)
	if err != nil {
		t.Fatalf("fs.exists failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("expected exists to return true for existing file")
	}

	vm.Set("noExistsPath", tmpDir+"/noexist.txt")
	result, err = vm.RunString(`mote.fs.exists(noExistsPath)`)
	if err != nil {
		t.Fatalf("fs.exists failed: %v", err)
	}
	if result.ToBoolean() {
		t.Error("expected exists to return false for non-existing file")
	}
}

func TestFSList(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(tmpDir+"/file1.txt", []byte(""), 0644)
	os.WriteFile(tmpDir+"/file2.txt", []byte(""), 0644)
	os.Mkdir(tmpDir+"/subdir", 0755)

	vm := goja.New()
	hctx := &Context{
		Ctx:         context.Background(),
		Logger:      zerolog.Nop(),
		ScriptName:  "test.js",
		ExecutionID: "test-123",
		Config: Config{
			AllowedPaths: []string{tmpDir},
			MaxWriteSize: 10 * 1024 * 1024,
		},
	}

	err := Register(vm, hctx)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	vm.Set("testDir", tmpDir)
	result, err := vm.RunString(`mote.fs.list(testDir)`)
	if err != nil {
		t.Fatalf("fs.list failed: %v", err)
	}

	exported := result.Export()
	var count int
	switch arr := exported.(type) {
	case []interface{}:
		count = len(arr)
	case []string:
		count = len(arr)
	default:
		t.Fatalf("unexpected type: %T", exported)
	}
	if count != 3 {
		t.Errorf("expected 3 entries, got %d", count)
	}
}

func TestFSPathNotAllowed(t *testing.T) {
	vm := goja.New()
	hctx := &Context{
		Ctx:         context.Background(),
		Logger:      zerolog.Nop(),
		ScriptName:  "test.js",
		ExecutionID: "test-123",
		Config: Config{
			AllowedPaths: []string{"/tmp/allowed"},
			MaxWriteSize: 10 * 1024 * 1024,
		},
	}

	err := Register(vm, hctx)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	_, err = vm.RunString(`mote.fs.read("/etc/passwd")`)
	if err == nil {
		t.Error("expected error for path outside allowlist")
	}
}

func TestLogMethods(t *testing.T) {
	vm := goja.New()
	hctx := &Context{
		Ctx:         context.Background(),
		Logger:      zerolog.Nop(),
		ScriptName:  "test.js",
		ExecutionID: "test-123",
		Config:      DefaultConfig(),
	}

	err := Register(vm, hctx)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	scripts := []string{
		`mote.log.debug("debug message")`,
		`mote.log.info("info message")`,
		`mote.log.warn("warn message")`,
		`mote.log.error("error message")`,
		`console.log("console log")`,
		`console.log("multiple", "args", 123)`,
	}

	for _, script := range scripts {
		_, err := vm.RunString(script)
		if err != nil {
			t.Errorf("script '%s' failed: %v", script, err)
		}
	}
}

func TestHTTPAllowlist(t *testing.T) {
	tests := []struct {
		url       string
		allowlist []string
		allowed   bool
	}{
		{"https://api.example.com/data", nil, true},
		{"https://api.example.com/data", []string{}, true},
		{"https://api.example.com/data", []string{"example.com"}, true},
		{"https://api.example.com/data", []string{"other.com"}, false},
	}

	for _, tt := range tests {
		result := isURLAllowed(tt.url, tt.allowlist)
		if result != tt.allowed {
			t.Errorf("isURLAllowed(%s, %v) = %v, want %v", tt.url, tt.allowlist, result, tt.allowed)
		}
	}
}
