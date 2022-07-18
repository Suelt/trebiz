package sign

import (
	"fmt"
	"testing"
)

func TestRsa(t *testing.T) {

	PrivateKey1, PublicKey1, err1 := GenKeys()
	_, PublicKey2, err2 := GenKeys()

	if err1 != nil || err2 != nil {
		panic(err1)
	}

	Msg := []byte("hhhhh")

	sig := SignWithPrikey(Msg, PrivateKey1)

	ok, err := VerifySignEd(Msg, PublicKey1, sig)
	if ok {
		fmt.Println("verify successfully")
	} else {
		t.Fatal(err)
	}

	okk, _ := VerifySignEd(Msg, PublicKey2, sig)
	if okk {
		t.Fatal("verify err")
	} else {
		fmt.Println("right")
	}

}
