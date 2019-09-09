package keymagic

import (
	"bufio"
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"github.com/quan-to/chevron"
	"github.com/quan-to/chevron/etc"
	"github.com/quan-to/chevron/keyBackend"
	"github.com/quan-to/chevron/models"
	"github.com/quan-to/chevron/openpgp"
	"github.com/quan-to/chevron/openpgp/armor"
	"github.com/quan-to/chevron/openpgp/packet"
	"github.com/quan-to/slog"
	"io"
	"io/ioutil"
	"path"
	"strings"
	"sync"
	"time"

	// Include MD5 hashing algorithm by default
	_ "crypto/md5"
	// Include SHA1 hashing algorithm by default
	_ "crypto/sha1"
	// Include SHA256 hashing algorithm by default
	_ "crypto/sha256"
	// Include SHA512 hashing algorithm by default
	_ "crypto/sha512"
	// Include RIPEMD160 hashing algorithm by default
	_ "golang.org/x/crypto/ripemd160"
)

const MinKeyBits = 2048 // Should be safe until we have decent Quantum Computers

type PGPManager struct {
	sync.Mutex
	KeysBase64Encoded    bool
	keyIdentity          map[string][]*openpgp.Identity
	decryptedPrivateKeys map[string]*packet.PrivateKey
	entities             map[string]*openpgp.Entity
	fp8to16              map[string]string
	subKeyToKey          map[string]string
	krm                  etc.KRMInterface
	kbkend               keyBackend.Backend
	log                  slog.Instance
}

func MakePGPManagerWithKRM(log slog.Instance, keyBackend keyBackend.Backend, krm etc.KRMInterface) etc.PGPInterface {
	if log == nil {
		log = slog.Scope("PGPMan")
	} else {
		log = log.SubScope("PGPMan")
	}

	if keyBackend == nil {
		log.Fatal("No keyBackend specified")
	}

	return &PGPManager{
		kbkend:               keyBackend,
		KeysBase64Encoded:    remote_signer.KeysBase64Encoded,
		keyIdentity:          make(map[string][]*openpgp.Identity),
		decryptedPrivateKeys: make(map[string]*packet.PrivateKey),
		entities:             make(map[string]*openpgp.Entity),
		fp8to16:              make(map[string]string),
		subKeyToKey:          make(map[string]string),
		krm:                  krm,
		log:                  log,
	}
}

func (pm *PGPManager) MinKeyBits() int {
	return MinKeyBits
}

func (pm *PGPManager) LoadKeys() {
	pm.log.Operation(slog.NOTE).Debug("LoadKeys()")
	pm.Lock()
	defer pm.Unlock()

	if remote_signer.OnDemandKeyLoad {
		pm.log.Warn("On Demand Key load enabled. Skipping loading keys.")
	} else {
		pm.log.Info("Loading keys from %s -> %s", pm.kbkend.Name(), pm.kbkend.Path())

		files, err := pm.kbkend.List()
		if err != nil {
			pm.log.Fatal("Error listing keys: %s", err)
		}

		keysLoaded := 0

		for _, file := range files {
			pm.log.Info("Loading key %s", file)
			keyData, m, err := pm.kbkend.Read(file)
			if err != nil {
				pm.log.Error("Error loading key %s: %s", file, err)
				continue
			}

			if pm.KeysBase64Encoded {
				b, err := base64.StdEncoding.DecodeString(keyData)
				if err != nil {
					pm.log.Error("Error base64 decoding %s: %s", file, err)
					continue
				}
				keyData = string(b)
			}

			err, kl := pm.LoadKeyWithMetadata(keyData, m)
			if err != nil {
				pm.log.Error("Error decoding key %s: %s", file, err)
				continue
			}

			keysLoaded += kl
		}

		pm.log.Info("Loaded %d private keys.", keysLoaded)
	}
}

func (pm *PGPManager) LoadKeyWithMetadata(armoredKey, metadata string) (error, int) {
	pm.log.Operation(slog.NOTE).Debug("LoadKeyWithMetadata(---, ---)")
	err, n := pm.LoadKey(armoredKey)

	if err != nil {
		return err, n
	}

	fp, err := remote_signer.GetFingerPrintFromKey(armoredKey)
	if err != nil {
		pm.log.Error("Cannot get fingerprint from key: %s", err)
		return nil, n
	}

	if metadata != "" {
		var meta map[string]string
		err = json.Unmarshal([]byte(metadata), &meta)
		if err != nil {
			pm.log.Error("Error decoding metadata: %s", err)
			return nil, n
		}

		if meta["password"] != "" {
			err = pm.unlockKey(fp, meta["password"])
			if err != nil {
				pm.log.Error("Cannot unlock key %s using metadata: %s", fp, err)
				return nil, n
			}
			pm.log.Debug("Key %s unlocked using metadata.", fp)
			return nil, n
		}
	}

	pm.log.Debug("No metadata for key %s. Skipping unlock...", fp)

	return nil, n
}

func (pm *PGPManager) SetKeysBase64Encoded(k bool) {
	pm.log.Operation(slog.NOTE).Debug("SetKeysBase64Encoded(%s)", k)
	pm.KeysBase64Encoded = k
}

func (pm *PGPManager) LoadKey(armoredKey string) (error, int) {
	pm.log.Operation(slog.NOTE).Debug("LoadKey(---)")
	keysLoaded := 0
	kr := strings.NewReader(armoredKey)
	keys, err := openpgp.ReadArmoredKeyRing(kr)
	if err != nil {
		return err, 0
	}

	for _, key := range keys {
		if key.PrimaryKey != nil { // Cache Public Key
			fp := remote_signer.ByteFingerPrint2FP16(key.PrimaryKey.Fingerprint[:])
			pm.log.Info("Loaded public key %s", fp)
			pm.krm.AddKey(key, true) // Add sticky public keys
			ids := make([]*openpgp.Identity, 0)
			for _, v := range key.Identities {
				// Get only first
				c := *v // copy
				ids = append(ids, &c)
			}
			pm.keyIdentity[fp] = ids
			pm.fp8to16[fp[8:]] = fp
			pm.entities[fp] = key
		}
		if key.PrivateKey != nil {
			fp := remote_signer.ByteFingerPrint2FP16(key.PrimaryKey.Fingerprint[:])
			pm.log.Info("Loaded private key %s", fp)

			for _, sub := range key.Subkeys {
				subKeyFp := remote_signer.IssuerKeyIdToFP16(sub.PublicKey.KeyId)
				pm.log.Info("	Loaded subkey %s for %s", subKeyFp, fp)
				pm.subKeyToKey[subKeyFp] = fp
			}

			pm.krm.AddKey(key, true) // Add sticky public keys

			keysLoaded++
		}
	}

	return nil, keysLoaded
}

func (pm *PGPManager) sanitizeFingerprint(fp string) string {
	if len(fp) > 16 {
		fp = fp[len(fp)-16:]
	}
	if len(fp) == 8 {
		fp = pm.fp8to16[fp]
	}
	if len(fp) != 16 {
		//pm.log.Fatal("Cannot find key or invalid fingerprint: %s", fp)
		return ""
	}

	return fp
}

func (pm *PGPManager) FixFingerPrint(fp string) string {
	pm.Lock()
	defer pm.Unlock()

	return pm.sanitizeFingerprint(fp)
}

func (pm *PGPManager) IsKeyLocked(fp string) bool {
	pm.Lock()
	defer pm.Unlock()

	fp = pm.sanitizeFingerprint(fp)
	return pm.decryptedPrivateKeys[fp] != nil
}

func (pm *PGPManager) unlockKey(fp, password string) error {
	fp = pm.sanitizeFingerprint(fp)
	_ = pm.LoadKeyFromKB(fp)

	ent := pm.entities[fp]

	if ent == nil {
		pm.log.Error("No such key with fingerprint %s", fp)
		return fmt.Errorf("no such key %s", fp)
	}

	pk := ent.PrivateKey

	if pk == nil {
		return errors.New(fmt.Sprintf("private key %s not found", fp))
	}

	vpk := *pk // Copy data, for safety (aka: not unlock key at encrypted keys list)

	err := vpk.Decrypt([]byte(password))

	if err != nil {
		return err
	}

	if remote_signer.AgentKeyFingerPrint == "" { // set default fingerprint
		pm.log.Warn("No Agent Key FingerPrint specified. Using %s", fp)
		remote_signer.AgentKeyFingerPrint = fp
	}

	if pm.decryptedPrivateKeys[fp] != nil {
		pm.log.Info("Key %s already unlocked.", fp)
		return nil
	}

	z := pm.entities[fp]

	for _, kz := range z.Subkeys {
		subkeyfp := remote_signer.IssuerKeyIdToFP16(kz.PublicKey.KeyId)
		pm.log.Info("		Decrypting subkey %s from %s", subkeyfp, fp)
		err := kz.PrivateKey.Decrypt([]byte(password))
		if err != nil {
			return err
		}
		pm.decryptedPrivateKeys[subkeyfp] = kz.PrivateKey
		pm.log.Debug("		Creating virtual entity for subkey %s from %s", subkeyfp, fp)
		pm.entities[subkeyfp] = remote_signer.CreateEntityFromKeys(fmt.Sprintf("Subkey for %s", fp), "", "", 0, kz.PublicKey, kz.PrivateKey)
	}

	pm.decryptedPrivateKeys[fp] = &vpk

	return nil
}

func (pm *PGPManager) UnlockKey(fp, password string) error {
	pm.log.Operation(slog.NOTE).Debug("UnlockKey(%s, ---)", fp)
	pm.Lock()
	defer pm.Unlock()

	return pm.unlockKey(fp, password)
}

func (pm *PGPManager) LoadKeyFromKB(fingerPrint string) error {
	pm.log.Info("Loading key %s", fingerPrint)

	if pm.decryptedPrivateKeys[fingerPrint] != nil || pm.entities[fingerPrint] != nil {
		pm.log.Warn("Key %s is already loaded", fingerPrint)
		return nil
	}

	keyData, m, err := pm.kbkend.Read(fingerPrint)
	if err != nil {
		return err
	}

	if pm.KeysBase64Encoded {
		b, err := base64.StdEncoding.DecodeString(keyData)
		if err != nil {
			return err
		}
		keyData = string(b)
	}

	err, _ = pm.LoadKeyWithMetadata(keyData, m)
	if err != nil {
		return err
	}

	return nil
}

func (pm *PGPManager) GetPrivateKeyInfo(fingerPrint string) *models.KeyInfo {
	pm.log.Operation(slog.NOTE).Debug("GetPrivateKeyInfo(%s)", fingerPrint)
	for k, e := range pm.entities {
		v := e.PrivateKey
		if v == nil {
			continue
		}

		if remote_signer.CompareFingerPrint(k, fingerPrint) {
			z, _ := v.BitLength()
			return &models.KeyInfo{
				FingerPrint:           k,
				Identifier:            remote_signer.SimpleIdentitiesToString(pm.keyIdentity[k]),
				Bits:                  int(z),
				ContainsPrivateKey:    true,
				PrivateKeyIsDecrypted: pm.decryptedPrivateKeys[k] != nil,
			}
		}
	}

	return nil
}

func (pm *PGPManager) GetLoadedPrivateKeys() []models.KeyInfo {
	pm.log.Operation(slog.NOTE).Debug("GetLoadedPrivateKeys()")
	keyInfos := make([]models.KeyInfo, 0)

	for k, e := range pm.entities {
		v := e.PrivateKey
		if v == nil {
			continue
		}

		z, _ := v.BitLength()
		keyInfo := models.KeyInfo{
			FingerPrint:           k,
			Identifier:            remote_signer.SimpleIdentitiesToString(pm.keyIdentity[k]),
			Bits:                  int(z),
			ContainsPrivateKey:    true,
			PrivateKeyIsDecrypted: pm.decryptedPrivateKeys[k] != nil,
		}
		keyInfos = append(keyInfos, keyInfo)
	}

	return keyInfos
}

func (pm *PGPManager) GetLoadedKeys() []models.KeyInfo {
	pm.log.Operation(slog.NOTE).Debug("GetLoadedKeys()")
	keyInfos := make([]models.KeyInfo, 0)

	for k, e := range pm.entities {
		z, _ := e.PrimaryKey.BitLength()
		keyInfo := models.KeyInfo{
			FingerPrint:           k,
			Identifier:            remote_signer.SimpleIdentitiesToString(pm.keyIdentity[k]),
			Bits:                  int(z),
			ContainsPrivateKey:    e.PrivateKey != nil,
			PrivateKeyIsDecrypted: pm.decryptedPrivateKeys[k] != nil,
		}
		keyInfos = append(keyInfos, keyInfo)
	}

	return keyInfos
}

func (pm *PGPManager) SaveKey(fingerPrint, armoredData string, password interface{}) error {
	pm.log.Operation(slog.NOTE).Debug("SaveKey(%s, ---, ---)", fingerPrint)
	filename := fmt.Sprintf("%s.key", fingerPrint)
	if pm.KeysBase64Encoded {
		filename = fmt.Sprintf("%s.b64", fingerPrint)
	}

	filePath := path.Join(remote_signer.PrivateKeyFolder, filename)

	pm.log.Info("Saving key at %s", filePath)

	data := []byte(armoredData)

	if pm.KeysBase64Encoded {
		pm.log.Debug("Base64 Encoding enabled. Encoding key.")
		data = []byte(base64.StdEncoding.EncodeToString(data))
	}
	metadataJson := ""
	if password != nil {
		metadata := map[string]string{}
		metadata["password"] = password.(string)
		mj, _ := json.Marshal(metadata)
		metadataJson = string(mj)
	}

	rd, rm, err := pm.kbkend.Read(fingerPrint)

	if rd == "" || rm == "" || rm != metadataJson || string(data) != rd || err != nil {
		return pm.kbkend.SaveWithMetadata(fingerPrint, string(data), metadataJson)
	}

	pm.log.Warn("Key %s already in KeyBackend. Skipping add.", fingerPrint)

	return nil
}

func (pm *PGPManager) SignData(fingerPrint string, data []byte, hashAlgorithm crypto.Hash) (string, error) {
	pm.log.Operation(slog.NOTE).Debug("SignData(%s, ---, %s)", fingerPrint, hashAlgorithm)
	fingerPrint = pm.sanitizeFingerprint(fingerPrint)
	pm.Lock()
	pk := pm.decryptedPrivateKeys[fingerPrint]

	if pk == nil {
		pm.Unlock()
		pm.log.Warn("Key %s not loaded. Trying to load from keybackend", fingerPrint)
		err := pm.LoadKeyFromKB(fingerPrint)
		if err != nil {
			return "", errors.New(fmt.Sprintf("key %s is not decrypt or not loaded", fingerPrint))
		}
		pm.Lock()
		pk = pm.decryptedPrivateKeys[fingerPrint]
	}

	if pk == nil {
		pm.Unlock()
		return "", errors.New(fmt.Sprintf("key %s is not decrypt or not loaded", fingerPrint))
	}

	vpk := *pk
	ent := *pm.entities[fingerPrint]
	ent.PrivateKey = &vpk
	pm.Unlock()

	d := bytes.NewReader(data)

	var b bytes.Buffer
	bw := bufio.NewWriter(&b)

	config := &packet.Config{
		DefaultHash: hashAlgorithm,
	}

	err := openpgp.ArmoredDetachSign(bw, &ent, d, config)
	if err != nil {
		return "", err
	}
	err = bw.Flush()
	if err != nil {
		return "", err
	}

	return b.String(), nil
}

func (pm *PGPManager) GetPublicKeyEntity(fingerPrint string) *openpgp.Entity {
	pm.log.Operation(slog.NOTE).Debug("GetPublicKeyEntity(%s)", fingerPrint)
	pm.Lock()
	defer pm.Unlock()
	fingerPrint = pm.sanitizeFingerprint(fingerPrint)

	ent := pm.entities[fingerPrint]

	if ent == nil {
		// Try fetch subkey
		subMaster := pm.subKeyToKey[fingerPrint]
		if len(subMaster) > 0 {
			ent = pm.entities[subMaster]
		} else {
			// Try PKS
			ent = pm.krm.GetKey(fingerPrint)
		}
	}

	if ent != nil {
		pm.entities[fingerPrint] = ent
	}

	return ent
}

func (pm *PGPManager) GetPublicKey(fingerPrint string) *packet.PublicKey {
	pm.log.Operation(slog.NOTE).Debug("GetPublicKey(%s)", fingerPrint)
	var pubKey *packet.PublicKey
	pm.Lock()
	defer pm.Unlock()
	pm.log.Debug("Sanitizing fingerprint %s", fingerPrint)
	fingerPrint = pm.sanitizeFingerprint(fingerPrint)
	pm.log.Debug("Sanitized %s", fingerPrint)

	ent := pm.entities[fingerPrint]

	if ent == nil {
		pm.log.Debug("Not found in local cache as direct fingerprint. Trying by subkey")
		// Try fetch subkey
		subMaster := pm.subKeyToKey[fingerPrint]
		if len(subMaster) > 0 {
			ent = pm.entities[subMaster]
			pubKey = ent.PrimaryKey
			pm.log.Note("Found as master key %s", fingerPrint)
		} else {
			// Try PKS
			pm.log.Await("Not found as subkey. Checking in KeyRingManager")
			ent = pm.krm.GetKey(fingerPrint)
			if ent == nil {
				pm.log.Operation(slog.DONE).Warn("Not found in KeyRingManager")
			} else {
				pm.log.Success("Found in Key Ring Manager")
			}
		}
	}

	if ent != nil {
		pubKey = ent.PrimaryKey
		pm.entities[fingerPrint] = ent
	}

	return pubKey
}

func (pm *PGPManager) GetSubKeys(fingerPrint string, decrypted bool) openpgp.EntityList {
	pm.log.Operation(slog.NOTE).Debug("GetSubKeys(%s, %v)", fingerPrint, decrypted)
	list := make([]*openpgp.Entity, 0)
	for k, v := range pm.subKeyToKey {
		if v == fingerPrint {
			ent := *pm.entities[k]
			if decrypted && pm.decryptedPrivateKeys[k] != nil {
				ent.PrivateKey = pm.decryptedPrivateKeys[k]
			}
			list = append(list, &ent)
		}
	}
	return list
}

func (pm *PGPManager) GetKey(fingerPrint string) *openpgp.Entity {
	pm.log.Operation(slog.NOTE).Debug("GetKey(%s)", fingerPrint)
	fingerPrint = pm.FixFingerPrint(fingerPrint)

	// Try directly
	_ = pm.LoadKeyFromKB(fingerPrint)
	decv := pm.entities[fingerPrint]
	if decv != nil {
		return decv
	}

	// Try subkeys
	subKeyMaster := pm.subKeyToKey[fingerPrint]
	if subKeyMaster != fingerPrint {
		return pm.GetKey(subKeyMaster)
	}

	return nil
}

func (pm *PGPManager) GetPrivate(fingerPrint string) openpgp.EntityList {
	pm.log.Operation(slog.NOTE).Debug("GetPrivate(%s)", fingerPrint)
	var ent openpgp.Entity
	fingerPrint = pm.FixFingerPrint(fingerPrint)

	// Try directly
	_ = pm.LoadKeyFromKB(fingerPrint)
	decv := pm.decryptedPrivateKeys[fingerPrint]
	if decv != nil {
		ent = *pm.entities[fingerPrint]
		ent.PrivateKey = pm.decryptedPrivateKeys[fingerPrint]
		keys := pm.GetSubKeys(fingerPrint, true)
		keys = append(keys, &ent)
		return keys
	}

	// Try subkeys
	subKeyMaster := pm.subKeyToKey[fingerPrint]
	if subKeyMaster != fingerPrint {
		return pm.GetPrivate(subKeyMaster)
	}

	return nil
}

func (pm *PGPManager) GetPublicKeyAscii(fingerPrint string) (string, error) {
	pm.log.Note("GetPublicKeyAscii(%s)", fingerPrint)
	key := ""
	pubKey := pm.GetPublicKey(fingerPrint)

	if pubKey == nil {
		return "", fmt.Errorf("not found")
	}

	ent := pm.GetPublicKeyEntity(fingerPrint)

	if ent != nil { // Try get full entity first
		serializedEntity := bytes.NewBuffer(nil)
		err := ent.Serialize(serializedEntity)

		if err != nil {
			return "", err
		}

		buf := bytes.NewBuffer(nil)
		headers := map[string]string{
			"Version": "GnuPG v2",
			"Comment": "Generated by Chevron",
		}

		w, err := armor.Encode(buf, openpgp.PublicKeyType, headers)
		if err != nil {
			return "", err
		}
		_, err = w.Write(serializedEntity.Bytes())
		if err != nil {
			return "", err
		}
		err = w.Close()
		if err != nil {
			return "", err
		}

		key = buf.String()
	} else if pubKey != nil { // If not, get just the public key
		serializedEntity := bytes.NewBuffer(nil)
		err := pubKey.Serialize(serializedEntity)

		if err != nil {
			return "", err
		}

		buf := bytes.NewBuffer(nil)
		headers := map[string]string{
			"Version": "GnuPG v2",
			"Comment": "Generated by Chevron",
		}

		w, err := armor.Encode(buf, openpgp.PublicKeyType, headers)
		if err != nil {
			return "", err
		}
		_, err = w.Write(serializedEntity.Bytes())
		if err != nil {
			return "", err
		}
		err = w.Close()
		if err != nil {
			return "", err
		}

		key = buf.String()
	}

	return key, nil
}

func (pm *PGPManager) GetPrivateKeyAscii(fingerPrint, password string) (string, error) {
	pm.log.Operation(slog.NOTE).Debug("GetPrivateKeyAscii(%s, ---)", fingerPrint)
	key := ""
	ent := pm.GetKey(fingerPrint)

	if ent != nil && ent.PrivateKey != nil { // Try get full entity first
		// Decrypt / Encrypt to initialize Signer
		err := ent.PrivateKey.Decrypt([]byte(password))

		if err != nil {
			return "", err
		}

		_ = ent.PrivateKey.Encrypt([]byte(password))

		serializedEntity := bytes.NewBuffer(nil)
		err = ent.SerializePrivate(serializedEntity, &packet.Config{
			DefaultHash: crypto.SHA512,
		})
		if err != nil {
			return "", err
		}

		buf := bytes.NewBuffer(nil)
		headers := map[string]string{
			"Version": "GnuPG v2",
			"Comment": "Generated by Chevron",
		}

		w, err := armor.Encode(buf, openpgp.PrivateKeyType, headers)
		if err != nil {
			return "", err
		}
		_, err = w.Write(serializedEntity.Bytes())
		if err != nil {
			return "", err
		}
		err = w.Close()
		if err != nil {
			return "", err
		}

		key = buf.String()
	} else {
		return "", fmt.Errorf("cannot find private key for %s", fingerPrint)
	}

	return key, nil
}

func (pm *PGPManager) VerifySignatureStringData(data string, signature string) (bool, error) {
	pm.log.Operation(slog.NOTE).Debug("VerifySignatureStringData(---, %s)", signature)
	return pm.VerifySignature([]byte(data), signature)
}

func (pm *PGPManager) VerifySignature(data []byte, signature string) (bool, error) {
	pm.log.Operation(slog.NOTE).Debug("VerifySignature(---, %s)", signature)
	var issuerKeyId uint64
	var publicKey *packet.PublicKey
	var fingerPrint string

	signature = remote_signer.SignatureFix(signature)
	b := bytes.NewReader([]byte(signature))
	block, err := armor.Decode(b)
	if err != nil {
		return false, err
	}

	if block.Type != openpgp.SignatureType {
		return false, errors.New("openpgp packet is not signature")
	}

	reader := packet.NewReader(block.Body)
	for {
		pkt, err := reader.Next()

		if err != nil {
			return false, err
		}

		switch sig := pkt.(type) {
		case *packet.Signature:
			if sig.IssuerKeyId == nil {
				return false, errors.New("signature doesn't have an issuer")
			}
			issuerKeyId = *sig.IssuerKeyId
			fingerPrint = remote_signer.IssuerKeyIdToFP16(issuerKeyId)
		case *packet.SignatureV3:
			issuerKeyId = sig.IssuerKeyId
			fingerPrint = remote_signer.IssuerKeyIdToFP16(issuerKeyId)
		default:
			return false, errors.New("non signature packet found")
		}

		if len(fingerPrint) == 16 {
			publicKey = pm.GetPublicKey(fingerPrint)
			if publicKey != nil {
				break
			}
		}
	}

	if publicKey == nil {
		return false, errors.New("cannot find public key to verify signature")
	}

	keyRing := make(openpgp.EntityList, 1)
	keyRing[0] = pm.entities[fingerPrint]

	dr := bytes.NewReader(data)
	sr := strings.NewReader(signature)

	_, err = openpgp.CheckArmoredDetachedSignature(keyRing, dr, sr)

	if err != nil {
		return false, err
	}

	return true, nil
}

func (pm *PGPManager) GenerateTestKey() (string, error) {
	pm.log.Operation(slog.NOTE).Debug("GenerateTestKey()")
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)

	if err != nil {
		return "", err
	}

	var cTimestamp = time.Now()

	pgpPubKey := packet.NewRSAPublicKey(cTimestamp, &privateKey.PublicKey)
	pgpPrivKey := packet.NewRSAPrivateKey(cTimestamp, privateKey)

	err = pgpPrivKey.Encrypt([]byte("1234"))

	if err != nil {
		return "", err
	}

	e := remote_signer.CreateEntityFromKeys("", "", "", 0, pgpPubKey, pgpPrivKey)

	serializedEntity := bytes.NewBuffer(nil)
	err = e.SerializePrivate(serializedEntity, &packet.Config{
		DefaultHash: crypto.SHA512,
	})

	if err != nil {
		return "", err
	}

	buf := bytes.NewBuffer(nil)
	headers := map[string]string{
		"Version": "GnuPG v2",
		"Comment": "Generated by Chevron",
	}

	w, err := armor.Encode(buf, openpgp.PrivateKeyType, headers)
	if err != nil {
		return "", err
	}
	_, err = w.Write(serializedEntity.Bytes())
	if err != nil {
		return "", err
	}
	err = w.Close()
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

func (pm *PGPManager) GeneratePGPKey(identifier, password string, numBits int) (string, error) {
	pm.log.Operation(slog.NOTE).Debug("GeneratePGPKey(%s, ---, %d)", identifier, numBits)
	if numBits < MinKeyBits {
		return "", errors.New(fmt.Sprintf("dont generate RSA keys with less than %d, its not safe. try use 3072 or higher", MinKeyBits))
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, numBits)

	if err != nil {
		return "", err
	}

	var cTimestamp = time.Now()

	pgpPubKey := packet.NewRSAPublicKey(cTimestamp, &privateKey.PublicKey)
	pgpPrivKey := packet.NewRSAPrivateKey(cTimestamp, privateKey)

	err = pgpPrivKey.Encrypt([]byte(password))

	if err != nil {
		return "", err
	}

	identifier, comment, email := remote_signer.ExtractIdentifierFields(identifier)

	if packet.HasInvalidCharacters(identifier) || packet.HasInvalidCharacters(comment) || packet.HasInvalidCharacters(email) {
		return "", fmt.Errorf("the identifier has invalid characters '(', ')', '<', '>'. If you're trying to use the full identifier format please check if its in the right format Name <email>")
	}

	e := remote_signer.CreateEntityFromKeys(identifier, comment, email, 0, pgpPubKey, pgpPrivKey)

	serializedEntity := bytes.NewBuffer(nil)
	err = e.SerializePrivate(serializedEntity, &packet.Config{
		DefaultHash: crypto.SHA512,
	})

	if err != nil {
		return "", err
	}

	buf := bytes.NewBuffer(nil)
	headers := map[string]string{
		"Version": "GnuPG v2",
		"Comment": "Generated by Chevron",
	}

	w, err := armor.Encode(buf, openpgp.PrivateKeyType, headers)
	if err != nil {
		return "", err
	}
	_, err = w.Write(serializedEntity.Bytes())
	if err != nil {
		return "", err
	}
	err = w.Close()
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

func (pm *PGPManager) Encrypt(filename, fingerPrint string, data []byte, dataOnly bool) (string, error) {
	pm.log.Operation(slog.NOTE).Debug("Encrypt(%s, %s, ---, %v)", filename, fingerPrint, dataOnly)
	var pubKey = pm.GetPublicKey(fingerPrint)

	if pubKey == nil {
		return "", fmt.Errorf("no public key for %s", fingerPrint)
	}
	fingerPrint = remote_signer.ByteFingerPrint2FP16(pubKey.Fingerprint[:])
	var entity *openpgp.Entity

	pm.Lock()
	entity = pm.entities[fingerPrint]
	pm.Unlock()

	buf := bytes.NewBuffer(nil)

	hints := &openpgp.FileHints{
		FileName: filename,
		IsBinary: true,
		ModTime:  time.Now(),
	}

	config := &packet.Config{
		DefaultHash:            crypto.SHA512,
		DefaultCipher:          packet.CipherAES256,
		DefaultCompressionAlgo: packet.CompressionZLIB,
		CompressionConfig: &packet.CompressionConfig{
			Level: 9,
		},
	}

	closer, err := openpgp.Encrypt(buf, []*openpgp.Entity{entity}, nil, hints, config)

	if err != nil {
		return "", err
	}

	_, err = closer.Write(data)

	if err != nil {
		return "", err
	}

	err = closer.Close()
	if err != nil {
		return "", err
	}

	encData := buf.Bytes()

	if dataOnly {
		return base64.StdEncoding.EncodeToString(encData), nil
	}

	buf = bytes.NewBuffer(nil)
	headers := map[string]string{
		"Version": "GnuPG v2",
		"Comment": "Generated by Chevron",
	}

	w, err := armor.Encode(buf, "PGP MESSAGE", headers)
	if err != nil {
		return "", err
	}
	_, err = w.Write(encData)
	if err != nil {
		return "", err
	}
	err = w.Close()
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

func (pm *PGPManager) Decrypt(data string, dataOnly bool) (*models.GPGDecryptedData, error) {
	pm.log.Operation(slog.NOTE).Debug("Decrypt(---, %v)", dataOnly)
	var err error
	var fps []string
	ret := &models.GPGDecryptedData{}

	if dataOnly {
		fps, err = remote_signer.GetFingerPrintsFromEncryptedMessageRaw(data)
	} else {
		fps, err = remote_signer.GetFingerPrintsFromEncryptedMessage(data)
	}

	if err != nil {
		return nil, err
	}

	if len(fps) == 0 {
		return nil, fmt.Errorf("no encrypted payloads found")
	}

	var decv *packet.PrivateKey
	var ent openpgp.Entity
	var subent *openpgp.Entity

	pm.LoadKeys()

	pm.Lock()
	for _, v := range fps {
		// Try directly
		_ = pm.LoadKeyFromKB(v)
		decv = pm.decryptedPrivateKeys[v]
		if decv != nil {
			ent = *pm.entities[v]
			break
		}

		// Try subkeys
		subKeyMaster := pm.subKeyToKey[v]
		if len(subKeyMaster) > 0 {
			_ = pm.LoadKeyFromKB(subKeyMaster)
			// Check if it is decrypted
			decv = pm.decryptedPrivateKeys[subKeyMaster]
			if decv != nil {
				ent = *pm.entities[subKeyMaster]
				subent = pm.entities[v]
				break
			}
		}
	}
	pm.Unlock()

	if decv == nil {
		return nil, fmt.Errorf("no unlocked key for decrypting packet")
	}

	keyRing := make(openpgp.EntityList, 1)
	ent.PrivateKey = decv
	keyRing[0] = &ent

	if subent != nil {
		keyRing[1] = subent
	}

	var rd io.Reader

	if dataOnly {
		d, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(d)
	} else {
		if remote_signer.IsASCIIArmored(data) {
			srd := strings.NewReader(data)
			p, err := armor.Decode(srd)
			if err != nil {
				return nil, err
			}

			rd = p.Body
		} else {
			rd = strings.NewReader(data)
		}
	}

	md, err := openpgp.ReadMessage(rd, keyRing, nil, nil)

	if err != nil {
		return nil, err
	}

	rawData, err := ioutil.ReadAll(md.LiteralData.Body)

	if err != nil {
		return nil, err
	}

	ret.FingerPrint = remote_signer.IssuerKeyIdToFP16(ent.PrimaryKey.KeyId)
	ret.Base64Data = base64.StdEncoding.EncodeToString(rawData)
	ret.Filename = md.LiteralData.FileName

	return ret, nil
}

func (pm *PGPManager) GetCachedKeys() []models.KeyInfo {
	pm.log.Operation(slog.NOTE).Debug("GetCachedKeys()")
	return pm.krm.GetCachedKeys()
}
