package geosite

import (
	"os"
	"testing"
)

func TestGeosite(t *testing.T) {
	geosite, err := os.Open("../../geosite.dat")
	if err != nil {
		t.Skip("no geosite found")
		return
	}
	matcher, err := LoadGeositeMatcher(geosite, "cn")
	if err != nil {
		t.Fatal(err)
	}
	if !matcher.Match("baidu.cn") {
		t.Fatal("match failed")
	}
}
