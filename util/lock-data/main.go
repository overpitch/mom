package main

/* Zips up the master of magic lbx files and encrypts the zip with a password,
 * so the web build can be hosted publicly without exposing the game data.
 * The web page (web/index.html) asks for the password and decrypts the data.
 *
 * File format: "MOM1" | salt (16 bytes) | nonce (12 bytes) | AES-256-GCM ciphertext
 * The key is PBKDF2-SHA256 of the password with the salt, 'iterations' rounds.
 *
 *   go run ./util/lock-data -data /path/to/lbx/files -out web/data.bin
 */

import (
    "archive/zip"
    "bufio"
    "bytes"
    "crypto/aes"
    "crypto/cipher"
    "crypto/pbkdf2"
    "crypto/rand"
    "crypto/sha256"
    "flag"
    "fmt"
    "log"
    "os"
    "path/filepath"
    "strings"
)

// must match web/index.html
const iterations = 600000

func zipLbxFiles(dir string) ([]byte, error) {
    paths, err := filepath.Glob(filepath.Join(dir, "*"))
    if err != nil {
        return nil, err
    }

    var out bytes.Buffer
    zipper := zip.NewWriter(&out)

    count := 0
    for _, path := range paths {
        if !strings.EqualFold(filepath.Ext(path), ".lbx") {
            continue
        }

        data, err := os.ReadFile(path)
        if err != nil {
            return nil, err
        }

        writer, err := zipper.Create(strings.ToUpper(filepath.Base(path)))
        if err != nil {
            return nil, err
        }
        _, err = writer.Write(data)
        if err != nil {
            return nil, err
        }
        count += 1
    }

    if count == 0 {
        return nil, fmt.Errorf("no lbx files found in %v", dir)
    }

    err = zipper.Close()
    if err != nil {
        return nil, err
    }

    log.Printf("Zipped %v lbx files, %v bytes", count, out.Len())
    return out.Bytes(), nil
}

func readPassword() (string, error) {
    password := os.Getenv("MOM_PASSWORD")
    if password == "" {
        fmt.Print("Password: ")
        line, err := bufio.NewReader(os.Stdin).ReadString('\n')
        if err != nil {
            return "", err
        }
        password = strings.TrimSpace(line)
    }

    if password == "" {
        return "", fmt.Errorf("empty password")
    }

    return password, nil
}

func encrypt(plain []byte, password string) ([]byte, error) {
    salt := make([]byte, 16)
    rand.Read(salt)

    key, err := pbkdf2.Key(sha256.New, password, salt, iterations, 32)
    if err != nil {
        return nil, err
    }

    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, err
    }

    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, err
    }

    nonce := make([]byte, gcm.NonceSize())
    rand.Read(nonce)

    out := []byte("MOM1")
    out = append(out, salt...)
    out = append(out, nonce...)
    return gcm.Seal(out, nonce, plain, nil), nil
}

func main() {
    log.SetFlags(log.Ldate | log.Lshortfile | log.Lmicroseconds)

    var dataPath string
    var outPath string
    flag.StringVar(&dataPath, "data", "", "directory containing the master of magic lbx files")
    flag.StringVar(&outPath, "out", "web/data.bin", "where to write the encrypted data")
    flag.Parse()

    if dataPath == "" {
        log.Fatalf("give the lbx directory with -data")
    }

    zipped, err := zipLbxFiles(dataPath)
    if err != nil {
        log.Fatalf("Unable to zip data: %v", err)
    }

    password, err := readPassword()
    if err != nil {
        log.Fatalf("Unable to read password: %v", err)
    }

    locked, err := encrypt(zipped, password)
    if err != nil {
        log.Fatalf("Unable to encrypt data: %v", err)
    }

    err = os.WriteFile(outPath, locked, 0644)
    if err != nil {
        log.Fatalf("Unable to write %v: %v", outPath, err)
    }

    log.Printf("Wrote %v", outPath)
}
