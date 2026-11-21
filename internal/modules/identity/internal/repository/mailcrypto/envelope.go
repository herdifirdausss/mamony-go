// Package mailcrypto stores only authenticated, encrypted email-proof
// bearer envelopes. Keys are injected by deployment code, never held in SQL.
package mailcrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

var ErrEnvelope = errors.New("email proof envelope unavailable")

// Keyring supports an active seal key plus bounded old keys for queue drain.
// The caller must obtain all raw keys from a secret manager outside this repo.
type Keyring struct {
	active string
	keys map[string]cipher.AEAD
}

func NewKeyring(active string, material map[string][]byte) (*Keyring, error) {
	if !validID(active) || len(material) == 0 || len(material) > 8 {
		return nil, ErrEnvelope
	}
	keys := make(map[string]cipher.AEAD, len(material))
	for id, raw := range material {
		if !validID(id) || len(raw) != 32 {
			return nil, ErrEnvelope
		}
		block, err := aes.NewCipher(raw)
		if err != nil {
			return nil, ErrEnvelope
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, ErrEnvelope
		}
		keys[id] = aead
	}
	if _, ok := keys[active]; !ok {
		return nil, ErrEnvelope
	}
	return &Keyring{active:active, keys:keys}, nil
}

func validID(id string) bool {
	if len(id)<1 || len(id)>32 {return false}
	for _, ch := range id {
		if !(ch>='a' && ch<='z' || ch>='A' && ch<='Z' || ch>='0' && ch<='9' || ch=='_' || ch=='-') {
			return false
		}
	}
	return true
}

func contextAAD(subjectID, challengeID string) ([]byte,error) {
	if len(subjectID)!=26 || len(challengeID)!=26 ||
		subjectID[:4]!="sub_" || challengeID[:4]!="vch_" {
		return nil, ErrEnvelope
	}
	return []byte("mamony:email-delivery-envelope:v1\x00"+subjectID+"\x00"+challengeID+"\x00EMAIL_VERIFY"),nil
}

func (k *Keyring) Seal(subjectID, challengeID, bearer string) (string, []byte, error) {
	if k==nil || len(bearer)!=43 {return "",nil,ErrEnvelope}
	aad,err:=contextAAD(subjectID,challengeID)
	if err!=nil {return "",nil,ErrEnvelope}
	b,err:=base64.RawURLEncoding.DecodeString(bearer)
	if err!=nil || len(b)!=32 || base64.RawURLEncoding.EncodeToString(b)!=bearer {
		return "",nil,ErrEnvelope
	}
	ciphertext,ok:=k.keys[k.active]
	if !ok {return "",nil,ErrEnvelope}
	nonce:=make([]byte,ciphertext.NonceSize())
	if _,err=io.ReadFull(rand.Reader,nonce);err!=nil{return "",nil,ErrEnvelope}
	encrypted:=ciphertext.Seal(nil,nonce,[]byte(bearer),aad)
	out:=append(nonce,encrypted...)
	return k.active,out,nil
}

// Open refuses a different subject, challenge, purpose, key or tampered blob.
// It is for the trusted delivery worker; the caller must not log its output.
func (k *Keyring) Open(subjectID,challengeID,keyID string,blob []byte)(string,error) {
	if k==nil || !validID(keyID) || len(blob)>256 {return "",ErrEnvelope}
	aad,err:=contextAAD(subjectID,challengeID)
	if err!=nil{return "",ErrEnvelope}
	aead,ok:=k.keys[keyID]
	if !ok || len(blob)<aead.NonceSize()+aead.Overhead(){return "",ErrEnvelope}
	plain,err:=aead.Open(nil,blob[:aead.NonceSize()],blob[aead.NonceSize():],aad)
	if err!=nil{return "",ErrEnvelope}
	b,err:=base64.RawURLEncoding.DecodeString(string(plain))
	if err!=nil || len(b)!=32 || base64.RawURLEncoding.EncodeToString(b)!=string(plain){return "",ErrEnvelope}
	return string(plain),nil
}
