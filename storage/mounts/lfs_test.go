package mounts

import (
	"strings"
	"testing"
)

const oid = "4d7a214614ab2935c943f9e0ff69d22eadbb8f32b1258daaa5e2ca24d17e2393"

func TestParseLFSPointer(t *testing.T) {
	good := "version https://git-lfs.github.com/spec/v1\noid sha256:" + oid + "\nsize 48213344\n"
	p, ok := ParseLFSPointer([]byte(good))
	if !ok || p.OID != oid || p.Size != 48213344 {
		t.Fatalf("ParseLFSPointer(good) = %+v, %v", p, ok)
	}
	for name, b := range map[string]string{
		"plain text":   "hello\n",
		"no oid":       "version https://git-lfs.github.com/spec/v1\nsize 3\n",
		"no size":      "version https://git-lfs.github.com/spec/v1\noid sha256:" + oid + "\n",
		"short oid":    "version https://git-lfs.github.com/spec/v1\noid sha256:abc\nsize 3\n",
		"other hash":   "version https://git-lfs.github.com/spec/v1\noid sha1:" + oid[:40] + "\nsize 3\n",
		"bad size":     "version https://git-lfs.github.com/spec/v1\noid sha256:" + oid + "\nsize -1\n",
		"version late": "oid sha256:" + oid + "\nversion https://git-lfs.github.com/spec/v1\nsize 3\n",
		"too large":    good + strings.Repeat("x", MaxLFSPointerSize),
	} {
		if p, ok := ParseLFSPointer([]byte(b)); ok {
			t.Errorf("%s: ParseLFSPointer = %+v, want not a pointer", name, p)
		}
	}
}
