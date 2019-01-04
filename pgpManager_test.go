package remote_signer

import (
	"crypto"
	"encoding/base64"
	"github.com/quan-to/remote-signer/SLog"
	"testing"
)

const testKeyFingerprint = "0016A9CA870AFA59"
const testKeyPassword = "I think you will never guess"

const testSignatureData = "huebr for the win!"
const testSignatureSignature = `-----BEGIN PGP SIGNATURE-----

wsFcBAABCgAQBQJcJMPWCRAAFqnKhwr6WQAA3kwQAB6pxQkN+5yMt0LSkpIcjeOS
UPqcMabEQlkD2HQrzisXlUZgllqP4jYAjFLCeErt0uu598LXO6pNTw7MnFQSfgcJ
dJF2S05GwI4k00mMNzCTn7PbJe3d96QwjbTeanoMAjHhypZKi/StbtkFpIa+t9WI
zm+EE5trFdZoE1SMOr5j85afDecl0DsGHEkKdmJ2mLK4ja3uaxsijtLd8d7mdI+Y
LbI8UnpGyWMLkK8FpjBm+BaVeNicUvqkt/LO3LwslbKAViKpdL6Gu5x7x6Q+tAyO
PZ6P6DQKjuGJl8aSv0eoKQ1TQz6vasBZNsYlasU0fM6dXny9XIucUD5sTsUpbMhw
uO/xap6i3mBtFpzSfQCo/23KHeQajXS23Al56iUr85jlSQ9+JvJhZFrU9NQa+ypq
Xi/IxrqTTvttVurXAVME1m06JirpiuD8fDdQTTboekaqLg8rXQ5eKqW0pAMIqHvf
aq97YCqxH4F3T2EE77v6D9iLnbx/+7EGHoCehTMUYiAIAhlo93Xf/hnj40Hl/N18
gYr2Yd/IYVsAoGH6AHrIyUykXgsK6RXiBy0Sa7LN14TMCnQYzG2AUvXCDf184YAQ
1obsUVANy+qxH4lwMbEoznEsAU0ppqLchX1Ixdru5/SEgSV13Qv34rMEHCdVy4Oe
1Jcr1AyB3KmDhw76PaBh
=D//n
-----END PGP SIGNATURE-----`

const testDecryptDataAscii = `-----BEGIN PGP MESSAGE-----
Version: GnuPG v2
Comment: Generated by Quanto Remote Signer

wcFMA6uJF6HKi88OARAADv3z9DhWy8UI/yf09QecNw//3foPoh3I1ZCEaywEXfB6
qvYTIHCdE0aKa/lMVVjv94YGog+EetedZZ6Ow6mBz0EjzQCemjgoHnq+69kf8y9V
6oUwBUUcZz6uxJsUA2y3mfvFXYvA6CWWk9H/RoJzwHO9Px7CNZHIWaAPcPP9bJkf
9VAISjWgCEHnH4O5uavrOqBaHwgtKvb/Ya3Cq/NDlpWcwxOHcBPxyml9Tcs6HA+7
dqnr3qhjeDzGYuyXQSnNI+ut37mCbISC4jniljsSWy5l1YkD/JfpMyMpN83BPbpJ
DP55rMBonzCQ+iV9Wyt0zUUrExuArjqyU/DKx1ZmoKWEv4EU6+BjutVZbc+sQIkk
lxP0E3bMLn73qm4JU6A1A9WqFd+ndP8hxSPb7EnqwvQ6A05NLl44kZ0JfULO4+a7
dhDPPlyGeur09Y0JZiA6k+uF1+dug52E5iW6ohBhki9SNG2Y6m1wez1gLDj7OBbp
upUEH488XKGNGN96DxlQxq2ujfozpMiRXN6IYy8ZqWskVInD5GNRw9n0BNomG1p4
abKUhK+YZR/B8rJCqm3wTGvCc5hrmnEcj944oNaSvWzfbAH81bl/8/as1IL616Hq
rOvKBs2YHJT51yw9U7ShzJTH/6GLuCaViq4d8Txi2a9JEpn3VOXv65ZiyLQSHrzS
4AHkXrdO3DB3CYC/qPoKyU0Z0OH2kOBn4GvhjZ/gK+MVrSDsHhivyODC4PLgauLN
9xqB4GDksZWWXn9uD3YAASYNjI95S+DE4Rd04C/kg0GzuOAMI1yFpV1OK12+heKg
nT5O4fndAA==
=FQ83
-----END PGP MESSAGE-----`

const testDecryptDataOnly = "wcFMA6uJF6HKi88OARAATyFPVauyY3PKircZ3AlTMd2Iy1/FNKVxSKg1jKBhGvPCUdRRMqaJXz4dsEWNZp//QQMN3cd3JqJhw/AEGJJUQglwnXO2bYCXj6/RzsgRKCbj9Ijo1Y33Rbu+3+huWluYEQnWBfkbhnjeIrNRXxGvqQKczXx1aA6D1CvFk8W5LUWmIngxKi+s2TxA/fqfMBETnKa6rVM625by/9Ebo7qKoeetksDYAMvEzaLwKFIQ6O+lQt1YcBnbZ3mkrtSosisRqfmndkffUGsEJJ/g16ZYhwlDUYUGj7O/mRb01edPFLQko0THpAUhT7GH4Cw939W4wqddHSxgz9pEJKt8TsOqry2oiRQ+Qus5ygyMrLp5jH6JrExgGf5dlNUOs6R1JXozWhLXSZo7+kBg4hTRkRmdSu2adNvsO8tF2qjCWd/M0p2HfLEKTdvYFh6+d93wOVDYMvXzUB7NDIGlhi6gXs/D8+Tw+ZkLpRm9iWdLO9YpFquI+964sxAz5E5iEOBipJGTVpyxsU959kQ6hJqT4EWiATYMnqpnG7hGkDfXlKcwCeDBDKsUi3KVLw4PbSnRZQh9JNFv2VhyF2zQKpXI4hyF0QZoef2OT4a7xTOdHkdkAes/fcDhr4dwvQfp0uPOH1C8LViO7bVKBFnj+zTVftI0pVJ8MV/BV0Y5ru1hcRXOp9vS4AHk3apKw1UvqtPqAcgnbNy1euHYYuBt4HXhvbrgWuO+bpowlkmIXeAx4CrgJOKtLADx4PPk454/jrek18yYVGg4AZvEDOBu4b0E4EzkNh+m6OARX0nP/ig4wqHxtuLvNoCX4WOWAA=="

var testData = []byte(testSignatureData)

var pgpMan *PGPManager

func init() {
	SLog.SetTestMode()

	PrivateKeyFolder = "."
	KeyPrefix = "testkey_"
	KeysBase64Encoded = false

	pgpMan = MakePGPManager()
	pgpMan.LoadKeys()

	err := pgpMan.UnlockKey(testKeyFingerprint, testKeyPassword)
	if err != nil {
		panic(err)
	}
}

// region Tests
func TestVerifySign(t *testing.T) {
	valid, err := pgpMan.VerifySignature(testData, testSignatureSignature)
	if err != nil || !valid {
		t.Errorf("Signature not valid or error found: %s", err)
	}

	valid, err = pgpMan.VerifySignatureStringData(testSignatureData, testSignatureSignature)
	if err != nil || !valid {
		t.Errorf("Signature not valid or error found: %s", err)
	}

	invalidTestData := []byte("huebr for the win!" + "makemeinvalid")

	valid, err = pgpMan.VerifySignature(invalidTestData, testSignatureSignature)

	if valid || err == nil {
		t.Error("A invalid test data passed to verify has been validated!")
	}
}

func TestSign(t *testing.T) {
	_, err := pgpMan.SignData(testKeyFingerprint, testData, crypto.SHA512)
	if err != nil {
		t.Error(err)
	}
}

func TestDecrypt(t *testing.T) {
	g, err := pgpMan.Decrypt(testDecryptDataAscii, false)
	if err != nil {
		t.Error(err)
	}

	gd, err := base64.StdEncoding.DecodeString(g.Base64Data)
	if err != nil {
		t.Error(err)
	}

	if string(gd) != testSignatureData {
		t.Errorf("Decrypted data does no match. Expected \"%s\" got \"%s\"", string(gd), testSignatureData)
	}

	g, err = pgpMan.Decrypt(testDecryptDataOnly, true)
	if err != nil {
		t.Error(err)
	}

	gd, err = base64.StdEncoding.DecodeString(g.Base64Data)
	if err != nil {
		t.Error(err)
	}

	if string(gd) != testSignatureData {
		t.Errorf("Decrypted data does no match. Expected \"%s\" got \"%s\"", string(gd), testSignatureData)
	}
}

func TestEncrypt(t *testing.T) {
	d, err := pgpMan.Encrypt("testing", testKeyFingerprint, testData, false)

	if err != nil {
		t.Error(err)
	}

	// region Test Decrypt
	g, err := pgpMan.Decrypt(d, false)
	if err != nil {
		t.Error(err)
	}

	gd, err := base64.StdEncoding.DecodeString(g.Base64Data)
	if err != nil {
		t.Error(err)
	}

	if string(gd) != testSignatureData {
		t.Errorf("Decrypted data does no match. Expected \"%s\" got \"%s\"", string(gd), testSignatureData)
	}
	// endregion
	d, err = pgpMan.Encrypt("testing", testKeyFingerprint, testData, true)

	if err != nil {
		t.Error(err)
	}

	// region Test Decrypt
	g, err = pgpMan.Decrypt(d, true)
	if err != nil {
		t.Error(err)
	}

	gd, err = base64.StdEncoding.DecodeString(g.Base64Data)
	if err != nil {
		t.Error(err)
	}

	if string(gd) != testSignatureData {
		t.Errorf("Decrypted data does no match. Expected \"%s\" got \"%s\"", string(gd), testSignatureData)
	}
	// endregion
}

func TestGenerateKey(t *testing.T) {
	key, err := pgpMan.GeneratePGPKey("HUE", testKeyPassword, MinKeyBits)

	if err != nil {
		t.Error(err)
	}

	// Load key
	err, _ = pgpMan.LoadKey(key)
	if err != nil {
		t.Error(err)
	}

	fp, _ := GetFingerPrintFromKey(key)

	t.Logf("Key Fingerprint: %s", fp)

	// Unlock Key
	err = pgpMan.UnlockKey(fp, testKeyPassword)
	if err != nil {
		t.Error(err)
	}

	// Try sign
	signature, err := pgpMan.SignData(fp, testData, crypto.SHA512)
	if err != nil {
		t.Error(err)
	}
	// Try verify
	valid, err := pgpMan.VerifySignature(testData, signature)
	if err != nil {
		t.Error(err)
	}
	if !valid {
		t.Error("Generated signature is not valid!")
	}
}

// endregion
// region Benchmarks
func BenchmarkSign(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := pgpMan.SignData(testKeyFingerprint, testData, crypto.SHA512)
		if err != nil {
			b.Error(err)
		}
	}
}
func BenchmarkVerifySignature(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := pgpMan.VerifySignature(testData, testSignatureSignature)
		if err != nil {
			b.Error(err)
		}
	}
}
func BenchmarkVerifySignatureStringData(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := pgpMan.VerifySignatureStringData(testSignatureData, testSignatureSignature)
		if err != nil {
			b.Error(err)
		}
	}
}
func BenchmarkEncryptASCII(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := pgpMan.Encrypt("", testKeyFingerprint, testData, false)
		if err != nil {
			b.Error(err)
		}
	}
}
func BenchmarkEncryptDataOnly(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := pgpMan.Encrypt("", testKeyFingerprint, testData, true)
		if err != nil {
			b.Error(err)
		}
	}
}
func BenchmarkKeyGenerate2048(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := pgpMan.GeneratePGPKey("", "123456789", 2048)
		if err != nil {
			b.Error(err)
		}
	}
}
func BenchmarkKeyGenerate3072(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := pgpMan.GeneratePGPKey("", "123456789", 3072)
		if err != nil {
			b.Error(err)
		}
	}
}
func BenchmarkKeyGenerate4096(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := pgpMan.GeneratePGPKey("", "123456789", 4096)
		if err != nil {
			b.Error(err)
		}
	}
}

// endregion
