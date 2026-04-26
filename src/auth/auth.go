package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/apimgr/pastebin/src/database"
	"github.com/apimgr/pastebin/src/models"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type contextKey string

const UserContextKey contextKey = "user"

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserExists         = errors.New("user already exists")
	ErrInvalidToken       = errors.New("invalid token")
)

type AuthService struct {
	db        database.DB
	jwtSecret []byte
	jwtExpiry time.Duration
}

type JWTClaims struct {
	UserID string `json:"user_id"`
	jwt.RegisteredClaims
}

func NewAuthService(db database.DB, jwtSecret string, jwtExpiry time.Duration) *AuthService {
	return &AuthService{
		db:        db,
		jwtSecret: []byte(jwtSecret),
		jwtExpiry: jwtExpiry,
	}
}

// HashPassword hashes a password using bcrypt
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(bytes), err
}

// CheckPassword verifies a password against a hash
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// Register creates a new user
func (s *AuthService) Register(username, email, password string) (*models.User, string, error) {
	// Check if user exists
	existingUser, _ := s.db.GetUserByUsername(username)
	if existingUser != nil {
		return nil, "", ErrUserExists
	}

	existingUser, _ = s.db.GetUserByEmail(email)
	if existingUser != nil {
		return nil, "", ErrUserExists
	}

	// Hash password
	hashedPassword, err := HashPassword(password)
	if err != nil {
		return nil, "", err
	}

	user := &models.User{
		Username: username,
		Email:    email,
		Password: hashedPassword,
	}

	if err := s.db.CreateUser(user); err != nil {
		return nil, "", err
	}

	// Generate JWT token
	token, err := s.generateToken(user.ID)
	if err != nil {
		return nil, "", err
	}

	return user, token, nil
}

// Login authenticates a user and returns a JWT token
func (s *AuthService) Login(identifier, password string) (*models.User, string, error) {
	user, err := s.db.GetUserByUsernameOrEmail(identifier)
	if err != nil {
		return nil, "", err
	}
	if user == nil {
		return nil, "", ErrInvalidCredentials
	}

	if !CheckPassword(password, user.Password) {
		return nil, "", ErrInvalidCredentials
	}

	token, err := s.generateToken(user.ID)
	if err != nil {
		return nil, "", err
	}

	return user, token, nil
}

// generateToken creates a new JWT token for a user
func (s *AuthService) generateToken(userID string) (string, error) {
	claims := JWTClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.jwtExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

// ValidateToken validates a JWT token and returns the user ID
func (s *AuthService) ValidateToken(tokenString string) (string, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return s.jwtSecret, nil
	})

	if err != nil {
		return "", err
	}

	if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
		return claims.UserID, nil
	}

	return "", ErrInvalidToken
}

// GetUserFromToken extracts user from JWT or API token
func (s *AuthService) GetUserFromToken(tokenString string) (*models.User, error) {
	// Try JWT first
	userID, err := s.ValidateToken(tokenString)
	if err == nil {
		return s.db.GetUserByID(userID)
	}

	// Try API token
	apiToken, err := s.db.GetTokenByValue(tokenString)
	if err != nil || apiToken == nil {
		return nil, ErrInvalidToken
	}

	return s.db.GetUserByID(apiToken.UserID)
}

// Middleware creates authentication middleware
func (s *AuthService) Middleware(required bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var user *models.User

			// Check Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader != "" {
				tokenString := strings.TrimPrefix(authHeader, "Bearer ")
				if tokenString != authHeader { // Had "Bearer " prefix
					user, _ = s.GetUserFromToken(tokenString)
				} else {
					// Maybe it's just the token directly
					user, _ = s.GetUserFromToken(authHeader)
				}
			}

			// Check for API token in query param
			if user == nil {
				if token := r.URL.Query().Get("token"); token != "" {
					user, _ = s.GetUserFromToken(token)
				}
			}

			if required && user == nil {
				http.Error(w, `{"error":"Authentication required"}`, http.StatusUnauthorized)
				return
			}

			// Add user to context
			if user != nil {
				ctx := context.WithValue(r.Context(), UserContextKey, user)
				r = r.WithContext(ctx)
			}

			next.ServeHTTP(w, r)
		})
	}
}

// GetUserFromContext gets the user from request context
func GetUserFromContext(r *http.Request) *models.User {
	if user, ok := r.Context().Value(UserContextKey).(*models.User); ok {
		return user
	}
	return nil
}
