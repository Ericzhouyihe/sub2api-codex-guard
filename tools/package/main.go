// Command package 生成发布者密钥、构建签名 .s2plugin 包并独立验签。
//
//	go run ./tools/package keygen -out F:/keys/my-publisher
//	go run ./tools/package build -signing-key F:/keys/my-publisher.private -key-id my-publisher-v1 -output dist
//	go run ./tools/package verify -package dist/x.s2plugin -public-key F:/keys/my-publisher.public -key-id my-publisher-v1
package main

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/codexguard/sub2api-codex-guard/internal/guard"
)

const binaryName = "codex-guard"

var defaultPlatforms = []string{"linux-amd64", "linux-arm64"}

type manifest struct {
	SchemaVersion int               `json:"schema_version"`
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Version       string            `json:"version"`
	Description   string            `json:"description,omitempty"`
	Author        string            `json:"author,omitempty"`
	Requires      json.RawMessage   `json:"requires"`
	Capabilities  json.RawMessage   `json:"capabilities"`
	Runtimes      map[string]rtPath `json:"runtimes"`
	UI            struct {
		Entrypoint string `json:"entrypoint"`
	} `json:"ui"`
	Files map[string]string `json:"files"`
}

type rtPath struct {
	Path string `json:"path"`
}

type signature struct {
	Algorithm string `json:"algorithm"`
	KeyID     string `json:"key_id"`
	Signature string `json:"signature"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: package keygen|build|verify [flags]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "keygen":
		err = keygen(os.Args[2:])
	case "build":
		err = build(os.Args[2:])
	case "verify":
		err = verifyCommand(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func keygen(args []string) error {
	flags := flag.NewFlagSet("keygen", flag.ExitOnError)
	out := flags.String("out", "", "密钥路径前缀，生成 <out>.private 与 <out>.public")
	_ = flags.Parse(args)
	if *out == "" {
		return errors.New("-out is required")
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o700); err != nil {
		return err
	}
	if _, err := os.Stat(*out + ".private"); err == nil {
		return errors.New("private key already exists; refusing to overwrite")
	}
	if err := os.WriteFile(*out+".private", []byte(base64.StdEncoding.EncodeToString(private.Seed())+"\n"), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(*out+".public", []byte(base64.StdEncoding.EncodeToString(public)+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Println("public key:", base64.StdEncoding.EncodeToString(public))
	return nil
}

func readPrivateKey(file string) (ed25519.PrivateKey, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errors.New("private key must be a base64 Ed25519 seed")
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

func build(args []string) error {
	flags := flag.NewFlagSet("build", flag.ExitOnError)
	signingKey := flags.String("signing-key", "", "Ed25519 私钥文件（放在仓库之外）")
	keyID := flags.String("key-id", "", "宿主 plugins.trusted_publishers 中的键名")
	output := flags.String("output", "dist", "输出目录")
	platformList := flags.String("platforms", strings.Join(defaultPlatforms, ","), "逗号分隔的 GOOS-GOARCH")
	unsigned := flags.Bool("unsigned", false, "仅用于本地 allow_unsigned 调试")
	_ = flags.Parse(args)
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	var private ed25519.PrivateKey
	if !*unsigned {
		if *signingKey == "" || *keyID == "" {
			return errors.New("-signing-key and -key-id are required (or -unsigned for local debugging)")
		}
		if abs, _ := filepath.Abs(*signingKey); strings.HasPrefix(strings.ToLower(abs), strings.ToLower(root)+string(filepath.Separator)) {
			return errors.New("keep the signing private key outside the source tree")
		}
		if private, err = readPrivateKey(*signingKey); err != nil {
			return err
		}
	}
	rawSource, err := os.ReadFile(filepath.Join(root, "manifest.source.json"))
	if err != nil {
		return err
	}
	var m manifest
	decoder := json.NewDecoder(bytes.NewReader(rawSource))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&m); err != nil {
		return fmt.Errorf("manifest.source.json: %w", err)
	}
	if m.ID != guard.PluginID {
		return fmt.Errorf("manifest id %q differs from runtime PluginID %q", m.ID, guard.PluginID)
	}
	if !regexp.MustCompile(`^[a-z0-9]+([._-][a-z0-9]+)+$`).MatchString(m.ID) {
		return errors.New("manifest id does not match the host pattern")
	}
	files := map[string][]byte{}
	m.Runtimes = map[string]rtPath{}
	tmp, err := os.MkdirTemp("", "s2plugin-build-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	for _, platform := range strings.Split(*platformList, ",") {
		platform = strings.TrimSpace(platform)
		goos, goarch, ok := strings.Cut(platform, "-")
		if !ok {
			return fmt.Errorf("invalid platform %q", platform)
		}
		name := binaryName
		if goos == "windows" {
			name += ".exe"
		}
		target := filepath.Join(tmp, platform, name)
		cmd := exec.Command("go", "build", "-trimpath", "-buildvcs=false",
			"-ldflags=-s -w -X github.com/codexguard/sub2api-codex-guard/internal/guard.Version="+m.Version,
			"-o", target, "./cmd/codex-guard")
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+goos, "GOARCH="+goarch)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("build %s: %w", platform, err)
		}
		data, err := os.ReadFile(target)
		if err != nil {
			return err
		}
		rel := path.Join("runtimes", platform, name)
		files[rel] = data
		m.Runtimes[platform] = rtPath{Path: rel}
		fmt.Printf("built %s (%d bytes)\n", rel, len(data))
	}
	err = filepath.WalkDir(filepath.Join(root, "ui"), func(file string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in ui: %s", file)
		}
		rel, _ := filepath.Rel(root, file)
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = data
		return nil
	})
	if err != nil {
		return err
	}
	if _, ok := files[m.UI.Entrypoint]; !ok {
		return fmt.Errorf("ui entrypoint %s missing", m.UI.Entrypoint)
	}
	m.Files = map[string]string{}
	for name, data := range files {
		sum := sha256.Sum256(data)
		m.Files[name] = hex.EncodeToString(sum[:])
	}
	manifestBytes, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	manifestBytes = append(manifestBytes, '\n')
	entries := map[string][]byte{"manifest.json": manifestBytes}
	for name, data := range files {
		entries[name] = data
	}
	if !*unsigned {
		sig := signature{Algorithm: "ed25519", KeyID: *keyID, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(private, manifestBytes))}
		raw, _ := json.MarshalIndent(sig, "", "  ")
		entries["signature.json"] = append(raw, '\n')
	}
	if err := os.MkdirAll(*output, 0o755); err != nil {
		return err
	}
	suffix := ""
	if *unsigned {
		suffix = "_unsigned"
	}
	packagePath := filepath.Join(*output, fmt.Sprintf("codex-guard_sub2api_v%s%s.s2plugin", m.Version, suffix))
	if err := writeZip(packagePath, entries); err != nil {
		return err
	}
	fmt.Println("package:", packagePath)
	if *unsigned {
		return nil
	}
	public := private.Public().(ed25519.PublicKey)
	encodedPublic := base64.StdEncoding.EncodeToString(public)
	trusted := fmt.Sprintf("# 合并到 Sub2API 宿主 config.yaml 的 plugins 节点下，保存后重启应用服务。\nplugins:\n  allow_unsigned: false\n  trusted_publishers:\n    %s: %q\n", *keyID, encodedPublic)
	if err := os.WriteFile(filepath.Join(*output, "trusted-publisher.yaml"), []byte(trusted), 0o644); err != nil {
		return err
	}
	if err := verify(packagePath, public, *keyID); err != nil {
		return fmt.Errorf("self-verify failed: %w", err)
	}
	sum, err := fileSHA256(packagePath)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(*output, "SHA256SUMS"), []byte(sum+"  "+filepath.Base(packagePath)+"\n"), 0o644)
}

func writeZip(file string, entries map[string][]byte) error {
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, name := range names {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: stamp}
		mode := os.FileMode(0o644)
		if strings.HasPrefix(name, "runtimes/") {
			mode = 0o755
		}
		header.SetMode(mode)
		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}
		if _, err := writer.Write(entries[name]); err != nil {
			return err
		}
	}
	if err := archive.Close(); err != nil {
		return err
	}
	return os.WriteFile(file, buffer.Bytes(), 0o644)
}

func fileSHA256(file string) (string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func verifyCommand(args []string) error {
	flags := flag.NewFlagSet("verify", flag.ExitOnError)
	pkg := flags.String("package", "", ".s2plugin 路径")
	publicFile := flags.String("public-key", "", "Base64 公钥文件")
	keyID := flags.String("key-id", "", "期望的 key_id")
	_ = flags.Parse(args)
	raw, err := os.ReadFile(*publicFile)
	if err != nil {
		return err
	}
	public, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(public) != ed25519.PublicKeySize {
		return errors.New("invalid public key")
	}
	if err := verify(*pkg, public, *keyID); err != nil {
		return err
	}
	fmt.Println("verified:", *pkg)
	return nil
}

// verify 复刻宿主安装时的关键检查：路径安全、文件集合一致、哈希、签名。
func verify(file string, public ed25519.PublicKey, keyID string) error {
	reader, err := zip.OpenReader(file)
	if err != nil {
		return err
	}
	defer reader.Close()
	contents := map[string][]byte{}
	for _, entry := range reader.File {
		name := entry.Name
		if strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || strings.Contains("/"+name+"/", "/../") {
			return fmt.Errorf("unsafe path %q", name)
		}
		if _, dup := contents[name]; dup {
			return fmt.Errorf("duplicate entry %q", name)
		}
		rc, err := entry.Open()
		if err != nil {
			return err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return err
		}
		contents[name] = data
	}
	manifestBytes, ok := contents["manifest.json"]
	if !ok {
		return errors.New("manifest.json missing")
	}
	var m manifest
	if err := json.Unmarshal(manifestBytes, &m); err != nil {
		return err
	}
	for name, want := range m.Files {
		data, ok := contents[name]
		if !ok {
			return fmt.Errorf("declared file missing: %s", name)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != want {
			return fmt.Errorf("hash mismatch: %s", name)
		}
	}
	for name := range contents {
		if name != "manifest.json" && name != "signature.json" {
			if _, ok := m.Files[name]; !ok {
				return fmt.Errorf("undeclared file: %s", name)
			}
		}
	}
	var sig signature
	if err := json.Unmarshal(contents["signature.json"], &sig); err != nil {
		return errors.New("signature.json missing or invalid")
	}
	decoded, err := base64.StdEncoding.DecodeString(sig.Signature)
	if err != nil || sig.Algorithm != "ed25519" || sig.KeyID != keyID || !ed25519.Verify(public, manifestBytes, decoded) {
		return errors.New("signature verification failed")
	}
	return nil
}
