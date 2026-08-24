package product

import "testing"

func TestDetectPlatform(t *testing.T) {
	cases := []struct {
		fixture string
		want    Platform
	}{
		{"woocommerce.html", PlatformWooCommerce},
		{"shopify.html", PlatformShopify},
		{"shopify-sold-out.html", PlatformShopify},
		{"prestashop.html", PlatformPrestaShop},
		{"opencart.html", PlatformOpenCart},
		{"not-a-product-article.html", PlatformUnknown},
		{"jsonld-basic.html", PlatformUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			doc := mustParseDoc(t, loadFixture(t, tc.fixture))
			if got := DetectPlatform(doc); got != tc.want {
				t.Errorf("DetectPlatform(%s) = %q, want %q", tc.fixture, got, tc.want)
			}
		})
	}
}
