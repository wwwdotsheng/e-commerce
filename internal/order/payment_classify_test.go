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
	// 对外统一系统错误，不暴露“卖家钱包缺失”这个内部原因。
	if err != errno.ErrInternalServer {
		t.Fatalf("public error = %v, want %v", err, errno.ErrInternalServer)
	}
	if !errors.Is(fmt.Errorf("%w: x", wallet.ErrSellerWalletMissing), wallet.ErrSellerWalletMissing) {
		t.Fatal("sentinel should stay unwrappable")
	}
}
