package aliyundeploy

import "testing"

func TestSelectNetworkPair(t *testing.T) {
	pairs := []networkPair{
		{VpcID: "vpc-a", VSwitchID: "vsw-1"},
		{VpcID: "vpc-a", VSwitchID: "vsw-2"},
	}

	vpcID, vswID, note, err := selectNetworkPair(pairs, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if vpcID != "vpc-a" || vswID != "vsw-1" {
		t.Fatalf("got vpc=%q vsw=%q", vpcID, vswID)
	}
	if note == "" {
		t.Fatal("expected override note for multiple pairs")
	}

	vpcID, vswID, note, err = selectNetworkPair(pairs, "vpc-a", "vsw-2")
	if err != nil || note != "" || vpcID != "vpc-a" || vswID != "vsw-2" {
		t.Fatalf("explicit vsw: vpc=%q vsw=%q note=%q err=%v", vpcID, vswID, note, err)
	}
}

func TestSelectKeyPairName(t *testing.T) {
	if got := selectKeyPairName([]string{"other", DefaultKeyPairName}, ""); got != DefaultKeyPairName {
		t.Fatalf("expected default name, got %q", got)
	}
	if got := selectKeyPairName([]string{"only-one"}, ""); got != "only-one" {
		t.Fatalf("expected only key, got %q", got)
	}
	if got := selectKeyPairName(nil, "custom"); got != "" {
		t.Fatalf("expected empty for custom preferred, got %q", got)
	}
}
