package plugin

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"
)

func TestUtf16LEToString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{
			name: "empty_data",
			data: []byte{},
			want: "",
		},
		{
			name: "nil_data",
			data: nil,
			want: "",
		},
		{
			name: "ascii_hello",
			data: []byte{'H', 0, 'e', 0, 'l', 0, 'l', 0, 'o', 0},
			want: "Hello",
		},
		{
			name: "single_ascii_char",
			data: []byte{'A', 0},
			want: "A",
		},
		{
			name: "ascii_numbers",
			data: []byte{'1', 0, '2', 0, '3', 0},
			want: "123",
		},
		{
			name: "ascii_with_space",
			data: []byte{'H', 0, 'i', 0, ' ', 0, '!', 0},
			want: "Hi !",
		},
		{
			name: "cyrillic_privet",
			data: []byte{
				0x1F, 0x04, // П (U+041F)
				0x40, 0x04, // р (U+0440)
				0x38, 0x04, // и (U+0438)
				0x32, 0x04, // в (U+0432)
				0x35, 0x04, // е (U+0435)
				0x42, 0x04, // т (U+0442)
			},
			want: "Привет",
		},
		{
			name: "mixed_ascii_and_non_ascii",
			data: []byte{
				'H', 0, // H
				'i', 0, // i
				' ', 0, // space
				0x1F, 0x04, // П
			},
			want: "Hi П",
		},
		{
			name: "euro_sign",
			data: []byte{0xAC, 0x20}, // € (U+20AC)
			want: "€",
		},
		{
			name: "latin1_extended_char_as_raw_byte",
			data: []byte{0xA9, 0x00}, // U+00A9 with high=0 treated as raw byte
			want: "\xa9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := utf16LEToString(tt.data)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReadAssemblyScriptString_ZeroPointer(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer func() {
		_ = r.Close(ctx)
	}()

	mod, err := r.NewHostModuleBuilder("test").
		NewFunctionBuilder().
		WithFunc(func() {}).
		Export("dummy").
		Instantiate(ctx)
	require.NoError(t, err)
	defer func() {
		_ = mod.Close(ctx)
	}()

	result := readAssemblyScriptString(mod, 0)
	assert.Equal(t, "", result)
}

func TestReadAssemblyScriptString_WithMemory(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer func() {
		_ = r.Close(ctx)
	}()

	// Minimal WASM module with 1 page of memory
	wasmBinary := []byte{
		0x00, 0x61, 0x73, 0x6d, // magic
		0x01, 0x00, 0x00, 0x00, // version
		// memory section
		0x05, 0x03, 0x01, 0x00, 0x01, // 1 memory with min=0, max=1
		// export section
		0x07, 0x0a, 0x01,
		0x06, 'm', 'e', 'm', 'o', 'r', 'y',
		0x02, 0x00,
	}

	mod, err := r.InstantiateWithConfig(ctx, wasmBinary, wazero.NewModuleConfig().WithName("memtest"))
	require.NoError(t, err)
	defer func() {
		_ = mod.Close(ctx)
	}()

	mem := mod.Memory()
	require.NotNil(t, mem)

	tests := []struct {
		name       string
		setupMem   func()
		ptr        uint32
		wantResult string
	}{
		{
			name: "valid_ascii_string",
			setupMem: func() {
				ptr := uint32(20)
				// "Hello" in UTF-16LE
				strData := []byte{'H', 0, 'e', 0, 'l', 0, 'l', 0, 'o', 0}
				byteLen := uint32(len(strData))
				// Write rtSize at ptr-4
				mem.WriteUint32Le(ptr-4, byteLen)
				// Write string data
				mem.Write(ptr, strData)
			},
			ptr:        20,
			wantResult: "Hello",
		},
		{
			name: "empty_string",
			setupMem: func() {
				ptr := uint32(100)
				// Write rtSize=0 at ptr-4
				mem.WriteUint32Le(ptr-4, 0)
			},
			ptr:        100,
			wantResult: "",
		},
		{
			name: "cyrillic_string",
			setupMem: func() {
				ptr := uint32(200)
				// "Привет" in UTF-16LE
				strData := []byte{
					0x1F, 0x04, // П
					0x40, 0x04, // р
					0x38, 0x04, // и
					0x32, 0x04, // в
					0x35, 0x04, // е
					0x42, 0x04, // т
				}
				byteLen := uint32(len(strData))
				mem.WriteUint32Le(ptr-4, byteLen)
				mem.Write(ptr, strData)
			},
			ptr:        200,
			wantResult: "Привет",
		},
		{
			name: "string_too_large",
			setupMem: func() {
				ptr := uint32(300)
				// Set rtSize to > 1MB
				mem.WriteUint32Le(ptr-4, 1024*1024+1)
			},
			ptr:        300,
			wantResult: "<string too large>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.setupMem()

			got := readAssemblyScriptString(mod, tt.ptr)

			assert.Equal(t, tt.wantResult, got)
		})
	}
}

func TestEnvHostLibrary_Instantiate(t *testing.T) {
	t.Parallel()
	t.Run("registers_env_module_successfully", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		r := wazero.NewRuntime(ctx)
		defer func() {
			_ = r.Close(ctx)
		}()

		lib := &EnvHostLibrary{}

		err := lib.Instantiate(ctx, r)

		require.NoError(t, err)
	})

	t.Run("double_instantiate_returns_error", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		r := wazero.NewRuntime(ctx)
		defer func() {
			_ = r.Close(ctx)
		}()

		lib := &EnvHostLibrary{}

		err := lib.Instantiate(ctx, r)
		require.NoError(t, err)

		err = lib.Instantiate(ctx, r)
		require.Error(t, err)
	})
}

func TestEnvHostLibrary_Abort(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer func() {
		_ = r.Close(ctx)
	}()

	// Minimal WASM module with 1 page of memory
	wasmBinary := []byte{
		0x00, 0x61, 0x73, 0x6d, // magic
		0x01, 0x00, 0x00, 0x00, // version
		// memory section
		0x05, 0x03, 0x01, 0x00, 0x01,
		// export section
		0x07, 0x0a, 0x01,
		0x06, 'm', 'e', 'm', 'o', 'r', 'y',
		0x02, 0x00,
	}

	mod, err := r.InstantiateWithConfig(ctx, wasmBinary, wazero.NewModuleConfig().WithName("aborttest"))
	require.NoError(t, err)
	defer func() {
		_ = mod.Close(ctx)
	}()

	mem := mod.Memory()
	require.NotNil(t, mem)

	t.Run("handles_valid_message_and_filename", func(t *testing.T) {
		t.Parallel()

		lib := &EnvHostLibrary{}

		// Setup message at ptr=20
		messagePtr := uint32(20)
		msgData := []byte{'e', 0, 'r', 0, 'r', 0} // "err"
		mem.WriteUint32Le(messagePtr-4, uint32(len(msgData)))
		mem.Write(messagePtr, msgData)

		// Setup filename at ptr=100
		filePtr := uint32(100)
		fileData := []byte{'t', 0, '.', 0, 't', 0, 's', 0} // "t.ts"
		mem.WriteUint32Le(filePtr-4, uint32(len(fileData)))
		mem.Write(filePtr, fileData)

		// Should not panic
		lib.abort(ctx, mod, messagePtr, filePtr, 42, 10)
	})

	t.Run("handles_zero_pointers", func(t *testing.T) {
		t.Parallel()

		lib := &EnvHostLibrary{}

		// Should not panic
		lib.abort(ctx, mod, 0, 0, 0, 0)
	})
}

func TestEnvHostLibrary_ConsoleLog(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer func() {
		_ = r.Close(ctx)
	}()

	// Minimal WASM module with 1 page of memory
	wasmBinary := []byte{
		0x00, 0x61, 0x73, 0x6d, // magic
		0x01, 0x00, 0x00, 0x00, // version
		// memory section
		0x05, 0x03, 0x01, 0x00, 0x01,
		// export section
		0x07, 0x0a, 0x01,
		0x06, 'm', 'e', 'm', 'o', 'r', 'y',
		0x02, 0x00,
	}

	mod, err := r.InstantiateWithConfig(ctx, wasmBinary, wazero.NewModuleConfig().WithName("logtest"))
	require.NoError(t, err)
	defer func() {
		_ = mod.Close(ctx)
	}()

	mem := mod.Memory()
	require.NotNil(t, mem)

	t.Run("handles_valid_message", func(t *testing.T) {
		t.Parallel()

		lib := &EnvHostLibrary{}

		// Setup message at ptr=20
		messagePtr := uint32(20)
		msgData := []byte{'H', 0, 'i', 0} // "Hi"
		mem.WriteUint32Le(messagePtr-4, uint32(len(msgData)))
		mem.Write(messagePtr, msgData)

		// Should not panic
		lib.consoleLog(ctx, mod, messagePtr)
	})

	t.Run("handles_zero_pointer", func(t *testing.T) {
		t.Parallel()

		lib := &EnvHostLibrary{}

		// Should not panic
		lib.consoleLog(ctx, mod, 0)
	})
}
