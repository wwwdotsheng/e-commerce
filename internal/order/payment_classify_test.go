package order

import (
	"errors"
	"fmt"
	"testing"

	"e-commerce/internal/wallet"
	"e-commerce/pkg/errno"
)

func TestClassifyPaymentErrorSellerWalletMissing(t *testing.T) {
	code, err := classifyPaymentError(fmt.Errorf("%w: seller abc", wallet.ErrSellerWalletMissing))
	if code != orderPaymentErrSellerWallet {
		t.Fatalf("error code = %q, want %q", code, orderPaymentErrSellerWallet)
	}
	if err != errno.ErrOrderPaymentSellerWallet {
		t.Fatalf("public error = %v, want %v", err, errno.ErrOrderPaymentSellerWallet)
	}
	if !errors.Is(fmt.Errorf("%w: x", wallet.ErrSellerWalletMissing), wallet.ErrSellerWalletMissing) {
		t.Fatal("sentinel should stay unwrappable")
	}
}
