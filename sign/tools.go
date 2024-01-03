package sign

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"go.dedis.ch/kyber/v3"
	"go.dedis.ch/kyber/v3/pairing/bn256"
	"go.dedis.ch/kyber/v3/share"
	"go.dedis.ch/kyber/v3/sign/bls"
	"go.dedis.ch/kyber/v3/sign/tbls"
)

func GenKeys() (ed25519.PrivateKey, ed25519.PublicKey, error) {
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		panic(err)
	}
	return privKey, pubKey, err
}

func dataHashByte(data []byte) ([]byte, error) {
	msgHash := sha256.New()
	_, err := msgHash.Write(data)
	if err != nil {
		panic(err)
	}
	msgHashSum := msgHash.Sum(nil)
	return msgHashSum, err
}

func SignWithPrikey(data []byte, priKey ed25519.PrivateKey) []byte {
	msgHashSum, err := dataHashByte(data)
	if err != nil {
		panic(err)
	}
	signature := ed25519.Sign(priKey, msgHashSum)

	return signature
}

func VerifySignEd(data []byte, publicKey ed25519.PublicKey, sig []byte) (bool, error) {

	msgHashSum, err := dataHashByte(data)
	if err != nil {
		panic(err)
	}
	ok := ed25519.Verify(publicKey, msgHashSum, sig)
	return ok, nil
}

func GenTSKeys(t, n int) ([]*share.PriShare, *share.PubPoly) {
	suite := bn256.NewSuite()
	secret := suite.G1().Scalar().Pick(suite.RandomStream())
	priPoly := share.NewPriPoly(suite.G2(), t, secret, suite.RandomStream())
	pubPoly := priPoly.Commit(suite.G2().Point().Base())
	shares := priPoly.Shares(n)
	return shares, pubPoly
}

// SignTSPartial generates the partial threshold signature.
func SignTSPartial(priKey *share.PriShare, data []byte) []byte {

	msgHashSum, err := dataHashByte(data)
	if err != nil {
		panic(err)
	}

	suite := bn256.NewSuite()
	partialSig, err := tbls.Sign(suite, priKey, msgHashSum)
	if err != nil {
		panic(err)
	}

	return partialSig
}

// AssembleIntactTSPartial assembles the intact threshold signature.
func AssembleIntactTSPartial(partialSigs [][]byte, pubPoly *share.PubPoly, data []byte, t, n int) []byte {

	msgHashSum, err := dataHashByte(data)
	if err != nil {
		panic(err)
	}

	suite := bn256.NewSuite()
	intactSig, err := tbls.Recover(suite, pubPoly, msgHashSum, partialSigs, t, n)
	if err != nil {
		panic(err)
	}
	return intactSig
}

// VerifyTS verifies the threshold signature.
func VerifyTS(pubPoly *share.PubPoly, data []byte, intactSig []byte) (bool, error) {

	msgHashSum, err := dataHashByte(data)
	if err != nil {
		panic(err)
	}

	suite := bn256.NewSuite()
	err = bls.Verify(suite, pubPoly.Commit(), msgHashSum, intactSig)
	if err != nil {
		return false, err
	}
	return true, nil
}

// encode encodes the data into bytes.
// Data can be of any type.
// Examples can be seen form the tests.
func encode(data interface{}) ([]byte, error) {
	buf := bytes.Buffer{}
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// decode decodes bytes into the data.
// Data should be passed in the format of a pointer to a type.
// Examples can be seen form the tests.
func decode(s []byte, data interface{}) error {
	dec := json.NewDecoder(bytes.NewReader(s))
	if err := dec.Decode(data); err != nil {
		return err
	}
	return nil
}

// TSPartialMarshalled defines an intermediate type to marshall the partial key,
// since private share of a threshold signature cannot be encoded as bytes directly.
type TSPartialMarshalled struct {
	I      int
	Binary []byte
}

// MarshallTSPartialKey marshalls the V field in the private share as bytes,
// and create the intermediate type: TSPartialMarshalled.
func MarshallTSPartialKey(priShare *share.PriShare) (*TSPartialMarshalled, error) {
	shareAsBytes, err := priShare.V.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return &TSPartialMarshalled{
		I:      priShare.I,
		Binary: shareAsBytes,
	}, nil
}

// UnMarshallTSPartialKey unmarshalls the V field in the private share from TSPartialMarshalled.Binary.
func UnMarshallTSPartialKey(par *TSPartialMarshalled) (*share.PriShare, error) {
	suite := bn256.NewSuite()
	shareDecoded := &share.PriShare{
		I: par.I,
		V: suite.G2().Scalar(),
	}
	err := shareDecoded.V.UnmarshalBinary(par.Binary)
	if err != nil {
		return nil, err
	}
	return shareDecoded, nil
}

// EncodeTSPartialKey encodes the private share of a threshold signature.
func EncodeTSPartialKey(priShare *share.PriShare) ([]byte, error) {
	mtspk, err := MarshallTSPartialKey(priShare)
	if err != nil {
		return nil, err
	}
	return encode(mtspk)
}

// DecodeTSPartialKey decodes the private share of a threshold signature.
func DecodeTSPartialKey(data []byte) (*share.PriShare, error) {
	var par TSPartialMarshalled
	if err := decode(data, &par); err != nil {
		return nil, err
	}
	return UnMarshallTSPartialKey(&par)
}

// EqualTSPartialKey compares if two private shares equal.
func EqualTSPartialKey(p1, p2 *share.PriShare) bool {
	suite := bn256.NewSuite()
	return p1.I == p2.I && bytes.Equal(p1.Hash(suite), p2.Hash(suite))
}

// TSPublicMarshalled defines an intermediate type to marshall the public key,
// since public key of a threshold signature cannot be encoded as bytes directly.
type TSPublicMarshalled struct {
	BaseBytes   []byte
	CommitBytes [][]byte
}

// MarshallTSPublicKey marshalls the V field in the private share as bytes,
// and create the intermediate type: TSPartialMarshalled.
func MarshallTSPublicKey(pubKey *share.PubPoly) (*TSPublicMarshalled, error) {
	base, commits := pubKey.Info()

	baseAsBytes, err := base.MarshalBinary()
	if err != nil {
		return nil, err
	}

	commitCount := len(commits)
	commitBytes := make([][]byte, commitCount)
	for i, commit := range commits {
		commitBytes[i], err = commit.MarshalBinary()
		if err != nil {
			return nil, err
		}
	}

	return &TSPublicMarshalled{
		BaseBytes:   baseAsBytes,
		CommitBytes: commitBytes,
	}, nil
}

// UnMarshallTSPublicKey unmarshalls the public key from TSPublicMarshalled.
func UnMarshallTSPublicKey(tspm *TSPublicMarshalled) (*share.PubPoly, error) {
	baseDecoded := bn256.NewSuite().G2().Point()
	err := baseDecoded.UnmarshalBinary(tspm.BaseBytes)
	if err != nil {
		return nil, err
	}
	commitsDecoded := make([]kyber.Point, len(tspm.CommitBytes))
	for i, cb := range tspm.CommitBytes {
		tmp := bn256.NewSuite().G2().Point()
		err = tmp.UnmarshalBinary(cb)
		if err != nil {
			return nil, err
		}
		commitsDecoded[i] = tmp
	}

	return share.NewPubPoly(bn256.NewSuite().G2(), baseDecoded, commitsDecoded), nil
}

// EncodeTSPublicKey encodes the public key of a threshold signature.
func EncodeTSPublicKey(pubkey *share.PubPoly) ([]byte, error) {
	tspm, err := MarshallTSPublicKey(pubkey)
	if err != nil {
		return nil, err
	}

	return encode(tspm)
}

// DecodeTSPublicKey decodes the public key of a threshold signature.
func DecodeTSPublicKey(data []byte) (*share.PubPoly, error) {
	var tspm TSPublicMarshalled
	if err := decode(data, &tspm); err != nil {
		return nil, err
	}
	return UnMarshallTSPublicKey(&tspm)
}
