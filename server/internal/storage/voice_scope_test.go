package storage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestVoiceRenderAndGeneralMediaAreDisjoint(t *testing.T) {
	ctx := context.Background()
	l := newLocal(t, time.Now())
	general, renders := GeneralMedia(l), VoiceRenders(l)
	if GeneralMedia(nil) != nil || VoiceRenders(nil) != nil {
		t.Fatal("unconfigured storage should remain disabled")
	}
	const public = "audio/content/reviewed/sample.wav"
	const governed = VoiceRenderPrefix + "v/prayer/m/attempt/hash.opus"
	if err := general.Upload(ctx, public, []byte("catalog"), nil); err != nil {
		t.Fatal(err)
	}
	if err := renders.Upload(ctx, governed, []byte("personal"), nil); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name             string
		objects          ObjectStorage
		permitted, other string
	}{{"general", general, public, governed}, {"governed", renders, governed, public}} {
		t.Run(tc.name, func(t *testing.T) {
			s := tc.objects
			if url, err := s.GenerateSignedURL(ctx, tc.permitted, time.Minute); err != nil || url == "" {
				t.Fatal("authorized signing failed", err)
			}
			if url, err := s.GenerateSignedURL(ctx, tc.other, time.Minute); err == nil || url != "" {
				t.Fatal("signed out-of-scope key")
			}
			if b, err := s.Download(ctx, tc.other); err == nil || b != nil {
				t.Fatal("read out-of-scope bytes")
			}
			if exists, err := s.Exists(ctx, tc.other); err != nil || exists {
				t.Fatal("revealed out-of-scope object")
			}
			if _, err := s.GetSize(ctx, tc.other); err == nil {
				t.Fatal("revealed out-of-scope size")
			}
			if _, err := s.GetMetadata(ctx, tc.other); err == nil {
				t.Fatal("revealed out-of-scope metadata")
			}
			if keys, err := s.List(ctx, "audio/"); err != nil || !slices.Equal(keys, []string{tc.permitted}) {
				t.Fatalf("scope list = %v, %v", keys, err)
			}
			if err := s.Upload(ctx, tc.other, nil, nil); err == nil {
				t.Fatal("overwrote out-of-scope object")
			}
			if err := s.Delete(ctx, tc.other); err == nil {
				t.Fatal("deleted out-of-scope object")
			}
		})
	}
	for _, key := range []string{"audio/./voices/v/take.wav", "audio/%76oices/v/take.wav", "audio/v*/take.wav", "audio/vo\nices/v/take.wav"} {
		if ValidKey(key) || VoiceSourceKey(key) || GeneralMediaKey(key) || VoiceRenderKey(key) {
			t.Errorf("noncanonical object name accepted: %q", key)
		}
	}
}

func TestGovernedAudioIsNotSharedCacheable(t *testing.T) {
	ctx := context.Background()
	l := newLocal(t, time.Now())
	api, pre := &fakeAPI{}, &fakePresigner{}
	s3 := newS3WithClients(api, pre, "bucket")
	for _, key := range []string{
		VoiceRenderPrefix + "voice/reflection/model/hash.wav", VoiceRenderPrefix + "voice/prayer/model/attempt/hash.mp3",
		"voice-private/fixture/source.wav", "bible/audio/fixture/item.m4a", "bible/offline/fixture/package.zip",
	} {
		if err := s3.Upload(ctx, key, []byte("fixture"), nil); err != nil {
			t.Fatal(err)
		}
		if aws.ToString(api.put.CacheControl) != "private, no-store" {
			t.Fatal("governed object upload marked cacheable")
		}
		if _, err := s3.GenerateSignedURL(ctx, key, time.Minute); err != nil || aws.ToString(pre.got.ResponseCacheControl) != "private, no-store" {
			t.Fatal("missing signed GET response override", err)
		}
		if err := l.Upload(ctx, key, []byte("fixture"), nil); err != nil {
			t.Fatal(err)
		}
		u, err := l.GenerateSignedURL(ctx, key, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			method, byteRange string
			status            int
			body              string
		}{
			{http.MethodGet, "", http.StatusOK, "fixture"},
			{http.MethodHead, "", http.StatusOK, ""},
			{http.MethodGet, "bytes=0-2", http.StatusPartialContent, "fix"},
		} {
			r := httptest.NewRequest(tc.method, u, nil)
			if tc.byteRange != "" {
				r.Header.Set("Range", tc.byteRange)
			}
			w := httptest.NewRecorder()
			l.Handler("/media").ServeHTTP(w, r)
			if w.Code != tc.status || w.Header().Get("Cache-Control") != "private, no-store" || w.Body.String() != tc.body {
				t.Errorf("%s %s %q: unexpected status (%d), cache header or fixture bytes", tc.method, key, tc.byteRange, w.Code)
			}
		}
	}
}
