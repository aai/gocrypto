// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/

// Package pkcs7 provides signing based on RFC 2315.
package pkcs7

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"io"
	"time"
)

// Sign data with cert & private key.
func Sign(r io.Reader, cert *x509.Certificate, priv *rsa.PrivateKey) ([]byte, error) {
	return Sign2(r, cert, priv, nil)
}

// Sign2 reader with a cert, private key, and intermediate cert.
func Sign2(r io.Reader, cert *x509.Certificate, priv *rsa.PrivateKey, intermediate *x509.Certificate) ([]byte, error) {
	hash := sha256.New()
	if _, err := io.Copy(hash, r); err != nil {
		return nil, err
	}
	messageDigest := hash.Sum(nil)

	raw := cert.Raw
	if intermediate != nil {
		raw = append(raw, intermediate.Raw...)
	}

	signedData := SignedData{
		Version: 1,
		DigestAlgorithms: []AlgorithmIdentifier{
			AlgorithmIdentifier{Algorithm: oidSHA256, Parameters: asn1.RawValue{Tag: 5}},
		},
		ContentInfo: ContentInfo{
			ContentType: oidPKCS7Data,
		},
		Certificates: asn1.RawValue{Class: 2, Tag: 0, Bytes: raw, IsCompound: true},
		SignerInfos: []SignerInfo{
			SignerInfo{
				Version: 1,
				SignedIdentifier: IssuerAndSerialNumber{
					Issuer:       asn1.RawValue{FullBytes: cert.RawIssuer},
					SerialNumber: cert.SerialNumber,
				},
				DigestAlgorithm: AlgorithmIdentifier{Algorithm: oidSHA256, Parameters: asn1.RawValue{Tag: 5}},
				AuthenticatedAttributes: Attributes{
					NewAttribute(oidPKCS9ContentType, oidPKCS7Data),
					NewAttribute(oidPKCS9SigningTime, time.Now().UTC()),
					NewAttribute(oidPKCS9MessageDigest, messageDigest),
				},
				DigestEncryptionAlgorithm: AlgorithmIdentifier{Algorithm: oidPKCS1RSAEncryption, Parameters: asn1.RawValue{Tag: 5}},
				EncryptedDigest:           nil, // We fill this in later
				UnauthenticatedAttributes: 0,
			},
		},
	}

	encodedAuthenticatedAttributes, err := asn1.Marshal(signedData.SignerInfos[0].AuthenticatedAttributes)
	if err != nil {
		return nil, err
	}

	// For the digest of the authenticated attributes, we need a
	// slightly different encoding.  Change the attributes from a
	// SEQUENCE to a SET.

	originalFirstByte := encodedAuthenticatedAttributes[0]
	encodedAuthenticatedAttributes[0] = 0x31

	hash = sha256.New()
	hash.Write(encodedAuthenticatedAttributes)
	attributesDigest := hash.Sum(nil)

	encodedAuthenticatedAttributes[0] = originalFirstByte

	encryptedDigest, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, attributesDigest)
	if err != nil {
		return nil, err
	}
	signedData.SignerInfos[0].EncryptedDigest = encryptedDigest

	encodedSignedData, err := asn1.Marshal(signedData)
	if err != nil {
		return nil, err
	}

	signedDataWrapper := SignedDataWrapper{
		Oid:        oidPKCS7SignedData,
		SignedData: asn1.RawValue{Class: 2, Tag: 0, Bytes: encodedSignedData, IsCompound: true},
	}

	return asn1.Marshal(signedDataWrapper)
}
