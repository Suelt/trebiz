package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"math/rand"
	"sort"
	"time"
)

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

func dataHashByte(data interface{}) ([]byte, error) {
	dataBytes, _ := encode(data)
	msgHash := sha256.New()
	_, err := msgHash.Write(dataBytes)
	if err != nil {
		panic(err)
	}
	msgHashSum := msgHash.Sum(nil)
	return msgHashSum, err
}

//calculate the nanosecond
func calTimeDuration(time1 int64) int64 {

	return time.Now().UnixNano() - time1

}

func getRandomNum() int {
	rand.Seed(int64(time.Now().Nanosecond()))
	return rand.Intn(10)
}

type RequestSNSlice []RequestSN

func (a RequestSNSlice) Len() int {
	return len(a)
}
func (a RequestSNSlice) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}
func (a RequestSNSlice) Less(i, j int) bool {
	return a[j] > a[i]
}

func GetMapkey(result map[RequestSN]RequestBatch) []RequestSN {

	keys := []RequestSN{}

	for key := range result {
		keys = append(keys, key)
	}
	// sort key
	sort.Sort(RequestSNSlice(keys))

	return keys
}
