// Copyright 2016 - 2020 The excelize Authors. All rights reserved. Use of
// this source code is governed by a BSD-style license that can be found in
// the LICENSE file.
//
// Package excelize providing a set of functions that allow you to write to
// and read from XLSX files. Support reads and writes XLSX file generated by
// Microsoft Excel™ 2007 and later. Support save file without losing original
// charts of XLSX. This library needs Go version 1.10 or later.

package excelize

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"hash"
	"strings"

	"github.com/richardlehane/mscfb"
	"golang.org/x/crypto/md4"
	"golang.org/x/crypto/ripemd160"
	"golang.org/x/text/encoding/unicode"
)

var (
	blockKey                   = []byte{0x14, 0x6e, 0x0b, 0xe7, 0xab, 0xac, 0xd0, 0xd6} // Block keys used for encryption
	packageOffset              = 8                                                      // First 8 bytes are the size of the stream
	packageEncryptionChunkSize = 4096
	iterCount                  = 50000
	cryptoIdentifier           = []byte{ // checking protect workbook by [MS-OFFCRYPTO] - v20181211 3.1 FeatureIdentifier
		0x3c, 0x00, 0x00, 0x00, 0x4d, 0x00, 0x69, 0x00, 0x63, 0x00, 0x72, 0x00, 0x6f, 0x00, 0x73, 0x00,
		0x6f, 0x00, 0x66, 0x00, 0x74, 0x00, 0x2e, 0x00, 0x43, 0x00, 0x6f, 0x00, 0x6e, 0x00, 0x74, 0x00,
		0x61, 0x00, 0x69, 0x00, 0x6e, 0x00, 0x65, 0x00, 0x72, 0x00, 0x2e, 0x00, 0x44, 0x00, 0x61, 0x00,
		0x74, 0x00, 0x61, 0x00, 0x53, 0x00, 0x70, 0x00, 0x61, 0x00, 0x63, 0x00, 0x65, 0x00, 0x73, 0x00,
		0x01, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00,
	}
	oleIdentifier = []byte{
		0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1,
	}
)

// Encryption specifies the encryption structure, streams, and storages are
// required when encrypting ECMA-376 documents.
type Encryption struct {
	KeyData       KeyData       `xml:"keyData"`
	DataIntegrity DataIntegrity `xml:"dataIntegrity"`
	KeyEncryptors KeyEncryptors `xml:"keyEncryptors"`
}

// KeyData specifies the cryptographic attributes used to encrypt the data.
type KeyData struct {
	SaltSize        int    `xml:"saltSize,attr"`
	BlockSize       int    `xml:"blockSize,attr"`
	KeyBits         int    `xml:"keyBits,attr"`
	HashSize        int    `xml:"hashSize,attr"`
	CipherAlgorithm string `xml:"cipherAlgorithm,attr"`
	CipherChaining  string `xml:"cipherChaining,attr"`
	HashAlgorithm   string `xml:"hashAlgorithm,attr"`
	SaltValue       string `xml:"saltValue,attr"`
}

// DataIntegrity specifies the encrypted copies of the salt and hash values
// used to help ensure that the integrity of the encrypted data has not been
// compromised.
type DataIntegrity struct {
	EncryptedHmacKey   string `xml:"encryptedHmacKey,attr"`
	EncryptedHmacValue string `xml:"encryptedHmacValue,attr"`
}

// KeyEncryptors specifies the key encryptors used to encrypt the data.
type KeyEncryptors struct {
	KeyEncryptor []KeyEncryptor `xml:"keyEncryptor"`
}

// KeyEncryptor specifies that the schema used by this encryptor is the schema
// specified for password-based encryptors.
type KeyEncryptor struct {
	XMLName      xml.Name     `xml:"keyEncryptor"`
	URI          string       `xml:"uri,attr"`
	EncryptedKey EncryptedKey `xml:"encryptedKey"`
}

// EncryptedKey used to generate the encrypting key.
type EncryptedKey struct {
	XMLName                    xml.Name `xml:"http://schemas.microsoft.com/office/2006/keyEncryptor/password encryptedKey"`
	SpinCount                  int      `xml:"spinCount,attr"`
	EncryptedVerifierHashInput string   `xml:"encryptedVerifierHashInput,attr"`
	EncryptedVerifierHashValue string   `xml:"encryptedVerifierHashValue,attr"`
	EncryptedKeyValue          string   `xml:"encryptedKeyValue,attr"`
	KeyData
}

// StandardEncryptionHeader structure is used by ECMA-376 document encryption
// [ECMA-376] and Office binary document RC4 CryptoAPI encryption, to specify
// encryption properties for an encrypted stream.
type StandardEncryptionHeader struct {
	Flags        uint32
	SizeExtra    uint32
	AlgID        uint32
	AlgIDHash    uint32
	KeySize      uint32
	ProviderType uint32
	Reserved1    uint32
	Reserved2    uint32
	CspName      string
}

// StandardEncryptionVerifier structure is used by Office Binary Document RC4
// CryptoAPI Encryption and ECMA-376 Document Encryption. Every usage of this
// structure MUST specify the hashing algorithm and encryption algorithm used
// in the EncryptionVerifier structure.
type StandardEncryptionVerifier struct {
	SaltSize              uint32
	Salt                  []byte
	EncryptedVerifier     []byte
	VerifierHashSize      uint32
	EncryptedVerifierHash []byte
}

// Decrypt API decrypt the CFB file format with ECMA-376 agile encryption and
// standard encryption. Support cryptographic algorithm: MD4, MD5, RIPEMD-160,
// SHA1, SHA256, SHA384 and SHA512 currently.
func Decrypt(raw []byte, opt *Options) (packageBuf []byte, err error) {
	doc, err := mscfb.New(bytes.NewReader(raw))
	if err != nil {
		return
	}
	encryptionInfoBuf, encryptedPackageBuf := extractPart(doc)
	mechanism, err := encryptionMechanism(encryptionInfoBuf)
	if err != nil || mechanism == "extensible" {
		return
	}
	switch mechanism {
	case "agile":
		return agileDecrypt(encryptionInfoBuf, encryptedPackageBuf, opt)
	case "standard":
		return standardDecrypt(encryptionInfoBuf, encryptedPackageBuf, opt)
	default:
		err = errors.New("unsupport encryption mechanism")
		break
	}
	return
}

// extractPart extract data from storage by specified part name.
func extractPart(doc *mscfb.Reader) (encryptionInfoBuf, encryptedPackageBuf []byte) {
	for entry, err := doc.Next(); err == nil; entry, err = doc.Next() {
		switch entry.Name {
		case "EncryptionInfo":
			buf := make([]byte, entry.Size)
			i, _ := doc.Read(buf)
			if i > 0 {
				encryptionInfoBuf = buf
				break
			}
		case "EncryptedPackage":
			buf := make([]byte, entry.Size)
			i, _ := doc.Read(buf)
			if i > 0 {
				encryptedPackageBuf = buf
				break
			}
		}
	}
	return
}

// encryptionMechanism parse password-protected documents created mechanism.
func encryptionMechanism(buffer []byte) (mechanism string, err error) {
	if len(buffer) < 4 {
		err = errors.New("unknown encryption mechanism")
		return
	}
	versionMajor, versionMinor := binary.LittleEndian.Uint16(buffer[0:2]), binary.LittleEndian.Uint16(buffer[2:4])
	if versionMajor == 4 && versionMinor == 4 {
		mechanism = "agile"
		return
	} else if (2 <= versionMajor && versionMajor <= 4) && versionMinor == 2 {
		mechanism = "standard"
		return
	} else if (versionMajor == 3 || versionMajor == 4) && versionMinor == 3 {
		mechanism = "extensible"
	}
	err = errors.New("unsupport encryption mechanism")
	return
}

// ECMA-376 Standard Encryption

// standardDecrypt decrypt the CFB file format with ECMA-376 standard encryption.
func standardDecrypt(encryptionInfoBuf, encryptedPackageBuf []byte, opt *Options) ([]byte, error) {
	encryptionHeaderSize := binary.LittleEndian.Uint32(encryptionInfoBuf[8:12])
	block := encryptionInfoBuf[12 : 12+encryptionHeaderSize]
	header := StandardEncryptionHeader{
		Flags:        binary.LittleEndian.Uint32(block[:4]),
		SizeExtra:    binary.LittleEndian.Uint32(block[4:8]),
		AlgID:        binary.LittleEndian.Uint32(block[8:12]),
		AlgIDHash:    binary.LittleEndian.Uint32(block[12:16]),
		KeySize:      binary.LittleEndian.Uint32(block[16:20]),
		ProviderType: binary.LittleEndian.Uint32(block[20:24]),
		Reserved1:    binary.LittleEndian.Uint32(block[24:28]),
		Reserved2:    binary.LittleEndian.Uint32(block[28:32]),
		CspName:      string(block[32:]),
	}
	block = encryptionInfoBuf[12+encryptionHeaderSize:]
	algIDMap := map[uint32]string{
		0x0000660E: "AES-128",
		0x0000660F: "AES-192",
		0x00006610: "AES-256",
	}
	algorithm := "AES"
	_, ok := algIDMap[header.AlgID]
	if !ok {
		algorithm = "RC4"
	}
	verifier := standardEncryptionVerifier(algorithm, block)
	secretKey, err := standardConvertPasswdToKey(header, verifier, opt)
	if err != nil {
		return nil, err
	}
	// decrypted data
	x := encryptedPackageBuf[8:]
	blob, err := aes.NewCipher(secretKey)
	if err != nil {
		return nil, err
	}
	decrypted := make([]byte, len(x))
	size := 16
	for bs, be := 0, size; bs < len(x); bs, be = bs+size, be+size {
		blob.Decrypt(decrypted[bs:be], x[bs:be])
	}
	return decrypted, err
}

// standardEncryptionVerifier extract ECMA-376 standard encryption verifier.
func standardEncryptionVerifier(algorithm string, blob []byte) StandardEncryptionVerifier {
	verifier := StandardEncryptionVerifier{
		SaltSize:          binary.LittleEndian.Uint32(blob[:4]),
		Salt:              blob[4:20],
		EncryptedVerifier: blob[20:36],
		VerifierHashSize:  binary.LittleEndian.Uint32(blob[36:40]),
	}
	if algorithm == "RC4" {
		verifier.EncryptedVerifierHash = blob[40:60]
	} else if algorithm == "AES" {
		verifier.EncryptedVerifierHash = blob[40:72]
	}
	return verifier
}

// standardConvertPasswdToKey generate intermediate key from given password.
func standardConvertPasswdToKey(header StandardEncryptionHeader, verifier StandardEncryptionVerifier, opt *Options) ([]byte, error) {
	encoder := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewEncoder()
	passwordBuffer, err := encoder.Bytes([]byte(opt.Password))
	if err != nil {
		return nil, err
	}
	key := hashing("sha1", verifier.Salt, passwordBuffer)
	for i := 0; i < iterCount; i++ {
		iterator := createUInt32LEBuffer(i)
		key = hashing("sha1", iterator, key)
	}
	var block int
	hfinal := hashing("sha1", key, createUInt32LEBuffer(block))
	cbRequiredKeyLength := int(header.KeySize) / 8
	cbHash := sha1.Size
	buf1 := bytes.Repeat([]byte{0x36}, 64)
	buf1 = append(standardXORBytes(hfinal, buf1[:cbHash]), buf1[cbHash:]...)
	x1 := hashing("sha1", buf1)
	buf2 := bytes.Repeat([]byte{0x5c}, 64)
	buf2 = append(standardXORBytes(hfinal, buf2[:cbHash]), buf2[cbHash:]...)
	x2 := hashing("sha1", buf2)
	x3 := append(x1, x2...)
	keyDerived := x3[:cbRequiredKeyLength]
	return keyDerived, err
}

// standardXORBytes perform XOR operations for two bytes slice.
func standardXORBytes(a, b []byte) []byte {
	r := make([][2]byte, len(a), len(a))
	for i, e := range a {
		r[i] = [2]byte{e, b[i]}
	}
	buf := make([]byte, len(a))
	for p, q := range r {
		buf[p] = q[0] ^ q[1]
	}
	return buf
}

// ECMA-376 Agile Encryption

// agileDecrypt decrypt the CFB file format with ECMA-376 agile encryption.
// Support cryptographic algorithm: MD4, MD5, RIPEMD-160, SHA1, SHA256, SHA384
// and SHA512.
func agileDecrypt(encryptionInfoBuf, encryptedPackageBuf []byte, opt *Options) (packageBuf []byte, err error) {
	var encryptionInfo Encryption
	if encryptionInfo, err = parseEncryptionInfo(encryptionInfoBuf[8:]); err != nil {
		return
	}
	// Convert the password into an encryption key.
	key, err := convertPasswdToKey(opt.Password, encryptionInfo)
	if err != nil {
		return
	}
	// Use the key to decrypt the package key.
	encryptedKey := encryptionInfo.KeyEncryptors.KeyEncryptor[0].EncryptedKey
	saltValue, err := base64.StdEncoding.DecodeString(encryptedKey.SaltValue)
	if err != nil {
		return
	}
	encryptedKeyValue, err := base64.StdEncoding.DecodeString(encryptedKey.EncryptedKeyValue)
	if err != nil {
		return
	}
	packageKey, err := crypt(false, encryptedKey.CipherAlgorithm, encryptedKey.CipherChaining, key, saltValue, encryptedKeyValue)
	// Use the package key to decrypt the package.
	return cryptPackage(false, packageKey, encryptedPackageBuf, encryptionInfo)
}

// convertPasswdToKey convert the password into an encryption key.
func convertPasswdToKey(passwd string, encryption Encryption) (key []byte, err error) {
	var b bytes.Buffer
	saltValue, err := base64.StdEncoding.DecodeString(encryption.KeyEncryptors.KeyEncryptor[0].EncryptedKey.SaltValue)
	if err != nil {
		return
	}
	b.Write(saltValue)
	encoder := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewEncoder()
	passwordBuffer, err := encoder.Bytes([]byte(passwd))
	if err != nil {
		return
	}
	b.Write(passwordBuffer)
	// Generate the initial hash.
	key = hashing(encryption.KeyData.HashAlgorithm, b.Bytes())
	// Now regenerate until spin count.
	for i := 0; i < encryption.KeyEncryptors.KeyEncryptor[0].EncryptedKey.SpinCount; i++ {
		iterator := createUInt32LEBuffer(i)
		key = hashing(encryption.KeyData.HashAlgorithm, iterator, key)
	}
	// Now generate the final hash.
	key = hashing(encryption.KeyData.HashAlgorithm, key, blockKey)
	// Truncate or pad as needed to get to length of keyBits.
	keyBytes := encryption.KeyEncryptors.KeyEncryptor[0].EncryptedKey.KeyBits / 8
	if len(key) < keyBytes {
		tmp := make([]byte, 0x36)
		key = append(key, tmp...)
		key = tmp
	} else if len(key) > keyBytes {
		key = key[:keyBytes]
	}
	return
}

// hashing data by specified hash algorithm.
func hashing(hashAlgorithm string, buffer ...[]byte) (key []byte) {
	var hashMap = map[string]hash.Hash{
		"md4":        md4.New(),
		"md5":        md5.New(),
		"ripemd-160": ripemd160.New(),
		"sha1":       sha1.New(),
		"sha256":     sha256.New(),
		"sha384":     sha512.New384(),
		"sha512":     sha512.New(),
	}
	handler, ok := hashMap[strings.ToLower(hashAlgorithm)]
	if !ok {
		return key
	}
	for _, buf := range buffer {
		handler.Write(buf)
	}
	key = handler.Sum(nil)
	return key
}

// createUInt32LEBuffer create buffer with little endian 32-bit unsigned
// integer.
func createUInt32LEBuffer(value int) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, uint32(value))
	return buf
}

// parseEncryptionInfo parse the encryption info XML into an object.
func parseEncryptionInfo(encryptionInfo []byte) (encryption Encryption, err error) {
	err = xml.Unmarshal(encryptionInfo, &encryption)
	return
}

// crypt encrypt / decrypt input by given cipher algorithm, cipher chaining,
// key and initialization vector.
func crypt(encrypt bool, cipherAlgorithm, cipherChaining string, key, iv, input []byte) (packageKey []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return input, err
	}
	stream := cipher.NewCBCDecrypter(block, iv)
	stream.CryptBlocks(input, input)
	return input, nil
}

// cryptPackage encrypt / decrypt package by given packageKey and encryption
// info.
func cryptPackage(encrypt bool, packageKey, input []byte, encryption Encryption) (outputChunks []byte, err error) {
	encryptedKey := encryption.KeyData
	var offset = packageOffset
	if encrypt {
		offset = 0
	}
	var i, start, end int
	var iv, outputChunk []byte
	for end < len(input) {
		start = end
		end = start + packageEncryptionChunkSize

		if end > len(input) {
			end = len(input)
		}
		// Grab the next chunk
		var inputChunk []byte
		if (end + offset) < len(input) {
			inputChunk = input[start+offset : end+offset]
		} else {
			inputChunk = input[start+offset : end]
		}

		// Pad the chunk if it is not an integer multiple of the block size
		remainder := len(inputChunk) % encryptedKey.BlockSize
		if remainder != 0 {
			inputChunk = append(inputChunk, make([]byte, encryptedKey.BlockSize-remainder)...)
		}
		// Create the initialization vector
		iv, err = createIV(encrypt, i, encryption)
		if err != nil {
			return
		}
		// Encrypt/decrypt the chunk and add it to the array
		outputChunk, err = crypt(encrypt, encryptedKey.CipherAlgorithm, encryptedKey.CipherChaining, packageKey, iv, inputChunk)
		if err != nil {
			return
		}
		outputChunks = append(outputChunks, outputChunk...)
		i++
	}
	return
}

// createIV create an initialization vector (IV).
func createIV(encrypt bool, blockKey int, encryption Encryption) ([]byte, error) {
	encryptedKey := encryption.KeyData
	// Create the block key from the current index
	blockKeyBuf := createUInt32LEBuffer(blockKey)
	var b bytes.Buffer
	saltValue, err := base64.StdEncoding.DecodeString(encryptedKey.SaltValue)
	if err != nil {
		return nil, err
	}
	b.Write(saltValue)
	b.Write(blockKeyBuf)
	// Create the initialization vector by hashing the salt with the block key.
	// Truncate or pad as needed to meet the block size.
	iv := hashing(encryptedKey.HashAlgorithm, b.Bytes())
	if len(iv) < encryptedKey.BlockSize {
		tmp := make([]byte, 0x36)
		iv = append(iv, tmp...)
		iv = tmp
	} else if len(iv) > encryptedKey.BlockSize {
		iv = iv[0:encryptedKey.BlockSize]
	}
	return iv, nil
}
