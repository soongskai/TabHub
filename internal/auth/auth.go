package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func CheckPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

type SessionManager struct {
	secret []byte
}

func NewSessionManager(secret string) *SessionManager {
	return &SessionManager{secret: []byte(secret)}
}

func (s *SessionManager) Sign(userID int64, expiresAt time.Time) string {
	payload := fmt.Sprintf("%d|%d", userID, expiresAt.Unix())
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payload))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	token := payload + "|" + signature
	return base64.RawURLEncoding.EncodeToString([]byte(token))
}

func (s *SessionManager) Parse(token string) (int64, time.Time, bool) {
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0, time.Time{}, false
	}
	parts := strings.Split(string(decoded), "|")
	if len(parts) != 3 {
		return 0, time.Time{}, false
	}
	payload := parts[0] + "|" + parts[1]
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payload))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(expected), []byte(parts[2])) != 1 {
		return 0, time.Time{}, false
	}
	userID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, time.Time{}, false
	}
	expiresUnix, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, time.Time{}, false
	}
	expiresAt := time.Unix(expiresUnix, 0)
	if time.Now().After(expiresAt) {
		return 0, time.Time{}, false
	}
	return userID, expiresAt, true
}
