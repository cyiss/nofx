package config

import "testing"

func TestRejectPublicJWTSecrets(t *testing.T) {
	for _, secret := range []string{"", "short", insecureDefaultJWTSecret, "your-jwt-secret-change-this-in-production"} {
		t.Run(secret, func(t *testing.T) {
			t.Setenv("JWT_SECRET", secret)
			if err := initConfig(); err == nil {
				t.Fatal("accepted missing, weak or publicly documented JWT secret")
			}
		})
	}
}

func TestTelemetryRequiresExplicitOptIn(t *testing.T) {
	t.Setenv("JWT_SECRET", "private-existing-signing-secret-32-bytes-long")
	for _, tc := range []struct {
		value string
		want  bool
	}{{"", false}, {"false", false}, {"garbage", false}, {"1", false}, {"true", true}, {"TRUE", true}} {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv("EXPERIENCE_IMPROVEMENT", tc.value)
			if err := initConfig(); err != nil {
				t.Fatal(err)
			}
			if global.ExperienceImprovement != tc.want {
				t.Fatalf("EXPERIENCE_IMPROVEMENT=%q enabled=%v, want %v", tc.value, global.ExperienceImprovement, tc.want)
			}
			if global.JWTSecret != "private-existing-signing-secret-32-bytes-long" {
				t.Fatal("changed existing JWT secret")
			}
		})
	}
}
