package awsdeploy

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/smithy-go"
)

const DefaultKeyPairName = "cloud-forge-default"

type EnsureKeyPairInput struct {
	KeyName        string
	PrivateKeyPath string
}

type EnsureKeyPairOutput struct {
	KeyName           string
	PrivateKeyPath    string
	CreatedPrivateKey bool
	ImportedKeyPair   bool
}

func DefaultPrivateKeyPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".cloud-forge", "keys", "aws", DefaultKeyPairName+".pem"), nil
}

func EnsureKeyPair(ctx context.Context, cfg Config, input EnsureKeyPairInput) (*EnsureKeyPairOutput, error) {
	keyName := strings.TrimSpace(input.KeyName)
	if keyName == "" {
		keyName = DefaultKeyPairName
	}

	privateKeyPath, err := resolvePrivateKeyPath(input.PrivateKeyPath)
	if err != nil {
		return nil, err
	}

	awsCfg, err := loadAWSConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	client := ec2.NewFromConfig(awsCfg)

	remoteExists, err := keyPairExists(ctx, client, keyName)
	if err != nil {
		return nil, err
	}
	localExists, err := regularFileExists(privateKeyPath)
	if err != nil {
		return nil, err
	}
	if remoteExists && !localExists {
		return nil, fmt.Errorf("AWS key pair %q already exists but local private key %s is missing; pass --key-name to use an existing key pair, --ssh-key none to disable SSH, or delete the AWS key pair and retry", keyName, privateKeyPath)
	}

	publicKey, createdPrivateKey, err := ensurePrivateKey(privateKeyPath, keyName)
	if err != nil {
		return nil, err
	}

	importedKeyPair := false
	if !remoteExists {
		if _, err := client.ImportKeyPair(ctx, &ec2.ImportKeyPairInput{
			KeyName:           awssdk.String(keyName),
			PublicKeyMaterial: []byte(publicKey),
		}); err != nil {
			if !isDuplicateKeyPairError(err) {
				return nil, fmt.Errorf("import EC2 key pair %q: %w", keyName, err)
			}
		} else {
			importedKeyPair = true
		}
	}

	return &EnsureKeyPairOutput{
		KeyName:           keyName,
		PrivateKeyPath:    privateKeyPath,
		CreatedPrivateKey: createdPrivateKey,
		ImportedKeyPair:   importedKeyPair,
	}, nil
}

// LocalKeyMaterial ensures an RSA private key exists at path and returns its SSH public key.
func LocalKeyMaterial(privateKeyPath, comment string) (publicKey string, created bool, err error) {
	return ensurePrivateKey(privateKeyPath, comment)
}

func resolvePrivateKeyPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return DefaultPrivateKeyPath()
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		path = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), string(os.PathSeparator)))
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve private key path %q: %w", path, err)
	}
	return abs, nil
}

type keyPairDescriber interface {
	DescribeKeyPairs(context.Context, *ec2.DescribeKeyPairsInput, ...func(*ec2.Options)) (*ec2.DescribeKeyPairsOutput, error)
}

func keyPairExists(ctx context.Context, client keyPairDescriber, keyName string) (bool, error) {
	out, err := client.DescribeKeyPairs(ctx, &ec2.DescribeKeyPairsInput{
		KeyNames: []string{keyName},
	})
	if err == nil {
		return len(out.KeyPairs) > 0, nil
	}
	if isKeyPairNotFoundError(err) {
		return false, nil
	}
	return false, fmt.Errorf("describe EC2 key pair %q: %w", keyName, err)
}

func regularFileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err == nil {
		if info.IsDir() {
			return false, fmt.Errorf("private key path %s is a directory", path)
		}
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("stat private key path %s: %w", path, err)
}

func ensurePrivateKey(path, comment string) (string, bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", false, fmt.Errorf("create private key directory: %w", err)
	}

	created := false
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		key, err := rsa.GenerateKey(rand.Reader, 3072)
		if err != nil {
			return "", false, fmt.Errorf("generate RSA private key: %w", err)
		}
		data = pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(key),
		})
		if err := os.WriteFile(path, data, 0600); err != nil {
			return "", false, fmt.Errorf("write private key %s: %w", path, err)
		}
		created = true
	} else if err != nil {
		return "", false, fmt.Errorf("read private key %s: %w", path, err)
	}

	if err := os.Chmod(path, 0600); err != nil {
		return "", false, fmt.Errorf("set private key permissions %s: %w", path, err)
	}

	key, err := parseRSAPrivateKey(data)
	if err != nil {
		return "", false, fmt.Errorf("load private key %s: %w", path, err)
	}
	return marshalSSHRSAAuthorizedKey(&key.PublicKey, comment), created, nil
}

func parseRSAPrivateKey(data []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("missing PEM block")
	}
	if block.Type == "RSA PRIVATE KEY" {
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	}
	if block.Type == "PRIVATE KEY" {
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("expected RSA private key, got %T", key)
		}
		return rsaKey, nil
	}
	return nil, fmt.Errorf("unsupported PEM block type %q", block.Type)
}

func marshalSSHRSAAuthorizedKey(pub *rsa.PublicKey, comment string) string {
	var payload bytes.Buffer
	writeSSHString(&payload, []byte("ssh-rsa"))
	writeSSHMPInt(&payload, big.NewInt(int64(pub.E)))
	writeSSHMPInt(&payload, pub.N)

	authorizedKey := "ssh-rsa " + base64.StdEncoding.EncodeToString(payload.Bytes())
	if strings.TrimSpace(comment) != "" {
		authorizedKey += " " + strings.TrimSpace(comment)
	}
	return authorizedKey
}

func writeSSHString(buf *bytes.Buffer, value []byte) {
	_ = binary.Write(buf, binary.BigEndian, uint32(len(value)))
	buf.Write(value)
}

func writeSSHMPInt(buf *bytes.Buffer, value *big.Int) {
	data := value.Bytes()
	if len(data) > 0 && data[0]&0x80 != 0 {
		data = append([]byte{0}, data...)
	}
	writeSSHString(buf, data)
}

func isKeyPairNotFoundError(err error) bool {
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && apiErr.ErrorCode() == "InvalidKeyPair.NotFound"
}

func isDuplicateKeyPairError(err error) bool {
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && apiErr.ErrorCode() == "InvalidKeyPair.Duplicate"
}
