package s3

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// Эталонные примеры из документации AWS «Signature Calculations for the
// Authorization Header» и «Authenticating Requests: Using Query Parameters».
func awsExampleClient() *Client {
	return &Client{Endpoint: "https://s3.amazonaws.com", Region: "us-east-1", Bucket: "examplebucket",
		AccessKey: "AKIAIOSFODNN7EXAMPLE", SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		now: func() time.Time { return time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC) }}
}

func TestSigV4GetObjectVector(t *testing.T) {
	c := awsExampleClient()
	u, _ := c.objectURL("test.txt")
	req, _ := http.NewRequest("GET", u.String(), nil)
	req.Header.Set("Range", "bytes=0-9")
	c.sign(req, emptySHA256, "range")
	auth := req.Header.Get("Authorization")
	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, " +
		"SignedHeaders=host;range;x-amz-content-sha256;x-amz-date, " +
		"Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if auth != want {
		t.Fatalf("Authorization:\n got %s\nwant %s", auth, want)
	}
}

func TestSigV4PresignVector(t *testing.T) {
	c := awsExampleClient()
	got, err := c.Presign("GET", "test.txt", 86400*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "https://examplebucket.s3.amazonaws.com/test.txt?") ||
		!strings.HasSuffix(got, "X-Amz-Signature=aeeed9bbccd4d02ee5c0109b86d86835f995330da4c265957d157751f604d404") {
		t.Fatalf("presigned URL: %s", got)
	}
}

func TestKeyEncoding(t *testing.T) {
	c := &Client{Endpoint: "http://minio:9000", Bucket: "media", PathStyle: true}
	u, _ := c.objectURL("videos/abc/720p 1.ts")
	if u.String() != "http://minio:9000/media/videos/abc/720p%201.ts" {
		t.Fatalf("url: %s", u)
	}
	if uriEncode("a+b=c/d~", true) != "a%2Bb%3Dc%2Fd~" {
		t.Fatal(uriEncode("a+b=c/d~", true))
	}
}
