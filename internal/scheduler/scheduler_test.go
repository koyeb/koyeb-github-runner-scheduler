package scheduler

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSchedulerAuth(t *testing.T) {

	// we're only testing Auth here, so let's fake handleAction
	type fakeAPI struct {
		API
		handleAction func(payload *WebHookPayload) error
	}

	testCases := map[string]struct{
		githubSecret string
		apiSecret string
		body string
		expectedResponseCode int
		disableAuthEnv string
	}{
		"secrets match": {
			githubSecret: "foobar",
			apiSecret: "foobar",
			body : `{"action":"foobar"}`,
			expectedResponseCode: http.StatusOK,
		},
		"secrets do not match": {
			githubSecret: "foobar",
			apiSecret: "",
			body : `{"action":"foobar"}`,
			expectedResponseCode: http.StatusUnauthorized,
		},
		"empty body": {
			githubSecret: "foobar",
			apiSecret: "foobar",
			body : ``,
			expectedResponseCode: http.StatusBadRequest,
		},
		"auth is disabled": {
			githubSecret: "",
			apiSecret: "",
			body : `{"action":"foobar"}`,
			expectedResponseCode: http.StatusOK,
			disableAuthEnv: "true",
		},
	}

	handleAction := func(payload *WebHookPayload) error {
		return nil
	}

	for name, tc := range testCases {
		tc := tc
		t.Run(name, func(t *testing.T) {
			t.Setenv("DISABLE_AUTH", tc.disableAuthEnv)
			api := API{
				apiSecret: tc.apiSecret,
			}
			fake := fakeAPI{
				API: api,
				handleAction:  handleAction,
			}
			
			mux := http.NewServeMux()
			mux.HandleFunc("/", fake.scheduler)
			srv := httptest.NewServer(mux)
			body := []byte(tc.body)
		
			hash := hmac.New(sha256.New, []byte(tc.githubSecret))
			hash.Write(body)
			sig := "sha256=" + hex.EncodeToString(hash.Sum(nil))
				
			client := http.DefaultClient
			req, _ := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader(body))
			req.Header.Add("X-Hub-Signature-256",sig)
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != tc.expectedResponseCode {
				t.Fatalf("Got: %d, want: %d", resp.StatusCode, tc.expectedResponseCode)
			}
		})
	}


	
	
}