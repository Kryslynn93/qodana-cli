/*
 * Copyright 2021-2023 JetBrains s.r.o.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package core

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRequestLicenseData(t *testing.T) {
	expectedLicense := "license data"
	rightToken := "token data"
	os.Setenv(qodanaLicenseRequestCooldownEnv, "2")
	os.Setenv(qodanaLicenseRequestTimeoutEnv, "6")
	for _, testData := range []struct {
		name           string
		delay          int
		failedAttempts int
		actualAttempts int
		token          string
		success        bool
	}{
		{
			name:           "perfect server, right key",
			delay:          0,
			token:          rightToken,
			failedAttempts: 0,
			actualAttempts: 1,
			success:        true,
		},
		{
			name:           "perfect server, wrong key",
			delay:          0,
			failedAttempts: 0,
			actualAttempts: 1,
			token:          "wrong",
			success:        false,
		},
		{
			name:           "lagging server, right key",
			delay:          getTimeout() / 2,
			failedAttempts: 0,
			actualAttempts: 1,
			token:          rightToken,
			success:        true,
		},
		{
			name:           "very lagging server, right key",
			delay:          getTimeout() * 2,
			failedAttempts: 0,
			actualAttempts: getAttempts(),
			token:          rightToken,
			success:        false,
		},
		{
			name:           "errors on server, right key",
			delay:          0,
			failedAttempts: getAttempts(),
			actualAttempts: getAttempts(),
			token:          rightToken,
			success:        false,
		},
		{
			name:           "couple errors on server, right key",
			delay:          0,
			failedAttempts: getAttempts() - 1,
			actualAttempts: getAttempts(),
			token:          rightToken,
			success:        true,
		},
		{
			name:           "couple errors on server, wrong key",
			delay:          0,
			failedAttempts: getAttempts() - 1,
			actualAttempts: getAttempts(),
			token:          "wrong",
			success:        false,
		},
	} {
		t.Run(testData.name, func(t *testing.T) {
			requestServed := 0
			svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestServed++
				if r.URL.Path != qodanaLicenseUri {
					t.Errorf("expected uri to be '%s' got '%s'", qodanaLicenseUri, r.URL.Path)
				}
				authHeader := r.Header.Get("Authorization")
				expectedAuth := fmt.Sprintf("Bearer %s", rightToken)
				time.Sleep(time.Duration(testData.delay) * time.Second)
				if requestServed <= testData.failedAttempts {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				if authHeader != expectedAuth {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				_, _ = fmt.Fprint(w, expectedLicense)
			}))
			defer svr.Close()

			res, err := requestLicenseData(svr.URL, testData.token)
			if err != nil {
				if testData.success {
					t.Errorf("requestLicenseData should failed")
				}
				return
			}
			if testData.actualAttempts != requestServed {
				t.Errorf("expected to be '%d' requests but was '%d'", testData.actualAttempts, requestServed)
			}
			license := strings.TrimSpace(string(res))
			if license != expectedLicense {
				t.Errorf("expected response to be '%s' got '%s'", expectedLicense, license)
			}
		})
	}
}

func TestExtractLicenseKey(t *testing.T) {
	for _, testData := range []struct {
		name        string
		data        string
		expectedKey string
	}{
		{
			name:        "just a key",
			data:        `{ "licenseKey": "key" }`,
			expectedKey: "key",
		},
		{
			name:        "empty json",
			data:        `{ }`,
			expectedKey: "",
		},
		{
			name:        "just unknown field",
			data:        `{ "unknownField": true }`,
			expectedKey: "",
		},
		{
			name: "unknown field and key",
			data: `{ 
						"unknownField": true,
						"licenseKey": "key"
					}`,
			expectedKey: "key",
		},
		{
			name:        "almost real world data",
			data:        `{"licenseId":"VA5HGQWQH6","licenseKey":"VA5HGQWQH6","expirationDate":"2023-07-31","licensePlan":"EAP_ULTIMATE_PLUS"}`,
			expectedKey: "VA5HGQWQH6",
		},
	} {
		t.Run(testData.name, func(t *testing.T) {
			key := extractLicenseKey([]byte(testData.data))
			if key != testData.expectedKey {
				t.Errorf("expected key to be '%s' got '%s'", key, testData.expectedKey)
			}
		})
	}
}

func TestSetupLicenseToken(t *testing.T) {
	for _, testData := range []struct {
		name       string
		token      string
		loToken    string
		resToken   string
		sendFus    bool
		sendReport bool
	}{
		{
			name:       "no key",
			token:      "",
			loToken:    "",
			resToken:   "",
			sendFus:    true,
			sendReport: false,
		},
		{
			name:       "with token",
			token:      "a",
			loToken:    "",
			resToken:   "a",
			sendFus:    true,
			sendReport: true,
		},
		{
			name:       "with license only token",
			token:      "",
			loToken:    "b",
			resToken:   "b",
			sendFus:    false,
			sendReport: false,
		},
		{
			name:       "both tokens",
			token:      "a",
			loToken:    "b",
			resToken:   "a",
			sendFus:    true,
			sendReport: true,
		},
	} {
		t.Run(testData.name, func(t *testing.T) {
			err := os.Setenv(qodanaLicenseOnlyToken, testData.loToken)
			if err != nil {
				t.Fatal(err)
			}
			err = os.Setenv(qodanaToken, testData.token)
			if err != nil {
				t.Fatal(err)
			}
			setupLicenseToken(&QodanaOptions{})

			if licenseToken.Token != testData.resToken {
				t.Errorf("expected token to be '%s' got '%s'", testData.resToken, licenseToken.Token)
			}

			sendFUS := licenseToken.isAllowedToSendFUS()
			if sendFUS != testData.sendFus {
				t.Errorf("expected allow FUS to be '%t' got '%t'", testData.sendFus, sendFUS)
			}

			toSendReports := licenseToken.isAllowedToSendReports()
			if toSendReports != testData.sendReport {
				t.Errorf("expected allow send report to be '%t' got '%t'", testData.sendReport, toSendReports)
			}

			err = os.Unsetenv(qodanaLicenseOnlyToken)
			if err != nil {
				t.Fatal(err)
			}

			err = os.Unsetenv(qodanaToken)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSetupLicense(t *testing.T) {
	Prod.Code = "QDJVM"
	Prod.EAP = false
	license := `{"licenseId":"VA5HGQWQH6","licenseKey":"VA5HGQWQH6","expirationDate":"2023-07-31","licensePlan":"EAP_ULTIMATE_PLUS"}`
	expectedKey := "VA5HGQWQH6"

	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, license)
	}))
	defer svr.Close()
	err := os.Setenv(qodanaLicenseEndpoint, svr.URL)
	if err != nil {
		t.Fatal(err)
	}
	setupLicense("token")

	licenseKey := os.Getenv(qodanaLicense)
	if licenseKey != expectedKey {
		t.Errorf("expected key to be '%s' got '%s'", expectedKey, licenseKey)
	}

	err = os.Unsetenv(qodanaLicenseEndpoint)
	if err != nil {
		t.Fatal(err)
	}

	err = os.Unsetenv(qodanaLicense)
	if err != nil {
		t.Fatal(err)
	}
}
