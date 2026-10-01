package cardmeta

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/u-diary/illuse-corp-onboarding/backend/internal/pngmeta"
)

func newSigner(t *testing.T, seed byte) *Signer {
	t.Helper()
	s, err := NewSigner(bytes.Repeat([]byte{seed}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSignAndVerify(t *testing.T) {
	s := newSigner(t, 1)
	id := Identity{EmployeeNumber: "000001", Birthdate: "2001-03-15"}
	got, err := s.Verify(s.Chunks(id))
	if err != nil || got != id {
		t.Fatalf("Verify = %+v, %v", got, err)
	}
}

func TestVerifyRejects(t *testing.T) {
	s := newSigner(t, 1)
	valid := s.Chunks(Identity{EmployeeNumber: "000001", Birthdate: "2001-03-15"})
	replace := func(key, value string) []pngmeta.Chunk {
		out := append([]pngmeta.Chunk(nil), valid...)
		for i := range out {
			if out[i].Key == key {
				out[i].Value = value
			}
		}
		return out
	}
	without := func(key string) []pngmeta.Chunk {
		var out []pngmeta.Chunk
		for _, c := range valid {
			if c.Key != key {
				out = append(out, c)
			}
		}
		return out
	}

	tests := map[string][]pngmeta.Chunk{
		"メタデータなし":    nil,
		"社員番号なし":     without(KeyEmployeeNumber),
		"生年月日なし":     without(KeyBirthdate),
		"署名なし":       without(KeySignature),
		"社員番号を書き換え":  replace(KeyEmployeeNumber, "000002"),
		"生年月日を書き換え":  replace(KeyBirthdate, "2001-03-16"),
		"署名を書き換え":    replace(KeySignature, "v1.00"),
		"社員番号の形式が違う": replace(KeyEmployeeNumber, "1"),
		"生年月日の形式が違う": replace(KeyBirthdate, "2001/03/15"),
		"別の鍵で署名":     newSigner(t, 2).Chunks(Identity{EmployeeNumber: "000001", Birthdate: "2001-03-15"}),
		"社員番号が重複":    append(append([]pngmeta.Chunk(nil), valid...), pngmeta.Chunk{Key: KeyEmployeeNumber, Value: "000002"}),
	}
	for name, chunks := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := s.Verify(chunks); err != ErrInvalid {
				t.Errorf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestLoadSigner(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}

	ok := write("ok", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\n")
	if _, err := LoadSigner(ok); err != nil {
		t.Errorf("正しい鍵: %v", err)
	}
	for name, content := range map[string]string{
		"short":  "0123456789abcdef",
		"nothex": "zz23456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	} {
		if _, err := LoadSigner(write(name, content)); err == nil {
			t.Errorf("%s: エラーにならない", name)
		}
	}
	if _, err := LoadSigner(filepath.Join(dir, "missing")); err == nil {
		t.Error("存在しないファイル: エラーにならない")
	}
}
