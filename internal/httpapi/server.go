// Package httpapi: HTTP-слой (маршруты, middleware, обработчики).
package httpapi

import (
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"socialnet/internal/auth"
	"socialnet/internal/av"
	"socialnet/internal/config"
	"socialnet/internal/mailer"
	"socialnet/internal/media"
	"socialnet/internal/realtime"
	"socialnet/internal/store"
	"socialnet/internal/webpush"
)

type Server struct {
	cfg    config.Config
	store  *store.Store
	tokens *auth.TokenIssuer
	hub    *realtime.Hub
	log    *slog.Logger

	msgLimiter    *ipRateLimiter // лимит сообщений на пользователя (антиспам)
	uploadLimiter *ipRateLimiter
	wsTickets     *wsTickets
	files         media.Storage

	mailer      mailer.Mailer
	box         *auth.SecretBox // шифрование секретов 2FA
	mfaLimiter  *ipRateLimiter  // попытки кодов 2FA и пароля на пользователя
	mailLimiter *ipRateLimiter  // повторная отправка писем
	vulnLimiter *ipRateLimiter  // сообщения об уязвимостях с одного IP

	vapid      *webpush.VAPID // nil — push выключен
	pushClient *http.Client
	pushHosts  []string // разрешённые push-сервисы (защита от SSRF)

	walletLimiter *ipRateLimiter // денежные операции на пользователя

	avStore *av.Store     // музыка и видео
	avSem   chan struct{} // не больше N одновременных ffmpeg в API
}

func New(cfg config.Config, st *store.Store, log *slog.Logger) (*Server, error) {
	files, err := media.NewDisk(cfg.MediaDir)
	if err != nil {
		return nil, err
	}
	avStore, err := av.NewStore(filepath.Join(cfg.MediaDir, "av"))
	if err != nil {
		return nil, err
	}
	box, err := auth.NewSecretBox(cfg.TOTPKey)
	if err != nil {
		return nil, err
	}
	var vapid *webpush.VAPID
	if cfg.VAPIDPublic != "" {
		if vapid, err = webpush.NewVAPID(cfg.VAPIDPublic, cfg.VAPIDPrivate, "mailto:"+cfg.SecurityContact); err != nil {
			return nil, err
		}
	}
	var m mailer.Mailer = &mailer.Log{Log: log}
	if cfg.SMTPHost != "" {
		m = &mailer.SMTP{Addr: cfg.SMTPHost + ":" + cfg.SMTPPort, User: cfg.SMTPUser,
			Password: cfg.SMTPPassword, From: cfg.MailFrom}
	}
	return &Server{
		cfg:    cfg,
		store:  st,
		tokens: auth.NewTokenIssuer(cfg.JWTSecret, cfg.AccessTTL),
		hub:    realtime.NewHub(),
		log:    log,

		msgLimiter:    newIPRateLimiter(1, 30),       // 60 в минуту, всплеск 30
		uploadLimiter: newIPRateLimiter(30.0/60, 20), // 30 фото в минуту
		wsTickets:     newWSTickets(),
		files:         files,

		mailer:      m,
		box:         box,
		mfaLimiter:  newIPRateLimiter(5.0/300, 5),  // 5 попыток за 5 минут
		mailLimiter: newIPRateLimiter(3.0/3600, 3), // 3 письма в час
		vulnLimiter: newIPRateLimiter(5.0/3600, 5), // 5 отчётов в час с IP

		vapid:      vapid,
		pushClient: webpush.SafeClient(),
		pushHosts:  webpush.DefaultHosts,

		walletLimiter: newIPRateLimiter(20.0/60, 20), // 20 операций в минуту

		avStore: avStore,
		avSem:   make(chan struct{}, 2),
	}, nil
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	if s.cfg.TrustProxy {
		// Берём IP клиента из X-Forwarded-For только за своим Nginx,
		// иначе любой сможет подставить чужой IP и обойти rate limit.
		r.Use(middleware.RealIP)
	}
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders)

	r.Get("/.well-known/security.txt", s.handleSecurityTxt)
	r.Get("/api/v1/openapi.json", s.handleOpenAPI)
	if !s.cfg.DocsDisabled {
		r.Handle("/docs/*", docsHandler())
		r.Get("/docs", http.RedirectHandler("/docs/", http.StatusMovedPermanently).ServeHTTP)
	}
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	perMin, burst := s.cfg.AuthRatePerMin, s.cfg.AuthRateBurst
	if perMin <= 0 {
		perMin = 10
	}
	if burst <= 0 {
		burst = 20
	}
	authLimiter := newIPRateLimiter(float64(perMin)/60, burst)

	// Обычные JSON-запросы: таймаут и лимит тела 1 MiB.
	r.Group(func(r chi.Router) {
		r.Use(deadlines(20 * time.Second))
		r.Use(middleware.Timeout(15 * time.Second))
		r.Use(limitBody(1 << 20))

		r.Route("/api/v1", func(r chi.Router) {
			r.Route("/auth", func(r chi.Router) {
				r.Use(authLimiter.middleware)
				r.Post("/register", s.handleRegister)
				r.Post("/login", s.handleLogin)
				r.Post("/refresh", s.handleRefresh)
				r.Post("/logout", s.handleLogout)
				r.Post("/2fa/verify", s.handleMFAVerify)
				r.Post("/email/verify", s.handleVerifyEmail)
				r.Post("/password/forgot", s.handleForgotPassword)
				r.Post("/password/reset", s.handleResetPassword)
			})

			r.Group(func(r chi.Router) {
				r.Use(s.requireAuth)

				// профиль и приватность
				r.Get("/users/me", s.handleMe)
				r.Patch("/users/me", s.handleUpdateMe)
				r.Patch("/users/me/privacy", s.handleUpdatePrivacy)
				r.Get("/users/by-username/{username}", s.handleGetUserByUsername)
				r.Get("/users/{id}", s.handleGetUser)

				// друзья и блокировки
				r.Post("/users/{id}/friend", s.handleFriendAdd)
				r.Delete("/users/{id}/friend", s.handleFriendRemove)
				r.Post("/users/{id}/friend/reject", s.handleFriendReject)
				r.Get("/users/{id}/friends", s.handleFriendsList)
				r.Get("/friends/requests", s.handleFriendRequests)
				r.Post("/users/{id}/block", s.handleBlock)
				r.Delete("/users/{id}/block", s.handleUnblock)
				r.Get("/blocks", s.handleBlockedList)

				// посты, лента, хэштеги
				r.Post("/posts", s.handleCreatePost)
				r.Get("/posts/{id}", s.handleGetPost)
				r.Patch("/posts/{id}", s.handleUpdatePost)
				r.Delete("/posts/{id}", s.handleDeletePost)
				r.Post("/posts/{id}/like", s.handleLike)
				r.Delete("/posts/{id}/like", s.handleUnlike)
				r.Get("/posts/{id}/comments", s.handleComments)
				r.Post("/posts/{id}/comments", s.handleAddComment)
				r.Delete("/comments/{id}", s.handleDeleteComment)
				r.Get("/comments/{id}/replies", s.handleReplies)
				r.Post("/posts/{id}/reactions", s.handleReact)
				r.Delete("/posts/{id}/reactions", s.handleUnlike)
				r.Get("/users/{id}/posts", s.handleWall)
				r.Get("/feed", s.handleFeed)
				r.Get("/hashtags/{tag}/posts", s.handleHashtag)

				// уведомления
				r.Get("/notifications", s.handleNotifications)
				r.Post("/notifications/read", s.handleNotificationsRead)

				// сообщения и реалтайм
				r.Post("/users/{id}/messages", s.handleSendMessage)
				r.Get("/conversations", s.handleConversations)
				r.Get("/conversations/{id}/messages", s.handleMessages)
				r.Post("/conversations/{id}/read", s.handleMarkRead)
				r.Post("/conversations/{id}/typing", s.handleTyping)
				r.Post("/conversations/{id}/messages", s.handleSendToConversation)
				r.Post("/chats", s.handleCreateChat)
				r.Patch("/conversations/{id}", s.handleRenameChat)
				r.Get("/conversations/{id}/members", s.handleChatMembers)
				r.Post("/conversations/{id}/members", s.handleAddChatMember)
				r.Delete("/conversations/{id}/members/{uid}", s.handleRemoveChatMember)
				r.Put("/conversations/{id}/members/{uid}/role", s.handleSetChatRole)
				r.Get("/voice/{vid}", s.handleServeVoice)
				r.Put("/users/me/keys", s.handleSetKey)
				r.Get("/users/{id}/keys", s.handleGetKey)
				r.Post("/ws/ticket", s.handleWSTicket)

				// фото
				r.Put("/users/me/avatar", s.handleSetAvatar)
				r.Delete("/media/{id}", s.handleDeleteMedia)

				// безопасность аккаунта
				r.Post("/users/me/email/verification", s.handleResendVerification)
				r.Post("/users/me/password", s.handleChangePassword)
				r.Post("/users/me/2fa/setup", s.handle2FASetup)
				r.Post("/users/me/2fa/enable", s.handle2FAEnable)
				r.Post("/users/me/2fa/disable", s.handle2FADisable)
				r.Post("/users/me/2fa/backup-codes", s.handleBackupCodes)
				r.Get("/sessions", s.handleSessions)
				r.Delete("/sessions/{sid}", s.handleRevokeSession)
				r.Post("/sessions/revoke-others", s.handleRevokeOtherSessions)
				r.Get("/security/events", s.handleSecurityEvents)

				// онлайн-статус и уведомления
				r.Post("/presence", s.handlePresence)
				r.Get("/users/me/notification-settings", s.handleGetNotificationSettings)
				r.Put("/users/me/notification-settings", s.handlePutNotificationSettings)
				r.Get("/push/vapid-public-key", s.handleVAPIDKey)
				r.Post("/push/subscriptions", s.handlePushSubscribe)
				r.Delete("/push/subscriptions", s.handlePushUnsubscribe)

				// кошелёк и подарки
				r.Get("/wallet", s.handleWallet)
				r.Post("/wallet/transfer", s.handleTransfer)
				r.Post("/wallet/daily-bonus", s.handleDailyBonus)
				r.Get("/gifts/catalog", s.handleGiftCatalog)
				r.Post("/users/{id}/gifts", s.handleSendGift)
				r.Get("/users/{id}/gifts", s.handleUserGifts)
				r.Post("/communities/{id}/donate", s.handleDonate)
				r.Get("/communities/{id}/wallet", s.handleCommunityWallet)
				r.Post("/communities/{id}/wallet/payout", s.handleCommunityPayout)

				// музыка
				r.Get("/tracks/{tid}", s.handleGetTrack)
				r.Delete("/tracks/{tid}", s.handleDeleteTrack)
				r.Post("/tracks/{tid}/my-music", s.handleAddMyMusic)
				r.Delete("/tracks/{tid}/my-music", s.handleRemoveMyMusic)
				r.Get("/users/{id}/music", s.handleUserMusic)
				r.Get("/users/{id}/playlists", s.handleUserPlaylists)
				r.Post("/playlists", s.handleCreatePlaylist)
				r.Get("/playlists/{id}", s.handleGetPlaylist)
				r.Patch("/playlists/{id}", s.handleRenamePlaylist)
				r.Delete("/playlists/{id}", s.handleDeletePlaylist)
				r.Post("/playlists/{id}/tracks", s.handleAddPlaylistTrack)
				r.Delete("/playlists/{id}/tracks/{tid}", s.handleRemovePlaylistTrack)

				// видео
				r.Get("/videos/{vid}", s.handleGetVideo)
				r.Patch("/videos/{vid}", s.handleUpdateVideo)
				r.Delete("/videos/{vid}", s.handleDeleteVideo)
				r.Get("/users/{id}/videos", s.handleUserVideos)

				// сообщества
				r.Post("/communities", s.handleCreateCommunity)
				r.Get("/communities", s.handleMyCommunities)
				r.Get("/communities/by-slug/{slug}", s.handleGetCommunityBySlug)
				r.Get("/communities/{id}", s.handleGetCommunity)
				r.Patch("/communities/{id}", s.handleUpdateCommunity)
				r.Post("/communities/{id}/join", s.handleJoinCommunity)
				r.Post("/communities/{id}/leave", s.handleLeaveCommunity)
				r.Get("/communities/{id}/members", s.handleCommunityMembers)
				r.Put("/communities/{id}/members/{userID}/role", s.handleSetMemberRole)
				r.Delete("/communities/{id}/members/{userID}", s.handleRemoveMember)
				r.Get("/communities/{id}/posts", s.handleCommunityWall)
				r.Get("/communities/{id}/requests", s.handleJoinRequests)
				r.Post("/communities/{id}/requests/{userID}/approve", s.handleResolveJoin(true))
				r.Post("/communities/{id}/requests/{userID}/reject", s.handleResolveJoin(false))
				r.Get("/communities/{id}/events", s.handleCommunityEvents)

				// истории
				r.Post("/stories", s.handleCreateStory)
				r.Get("/stories/feed", s.handleStoriesFeed)
				r.Get("/users/{id}/stories", s.handleUserStories)
				r.Post("/stories/{id}/view", s.handleViewStory)
				r.Get("/stories/{id}/viewers", s.handleStoryViewers)
				r.Delete("/stories/{id}", s.handleDeleteStory)

				// мероприятия
				r.Post("/events", s.handleCreateEvent)
				r.Get("/events", s.handleUpcomingEvents)
				r.Get("/events/{id}", s.handleGetEvent)
				r.Patch("/events/{id}", s.handleUpdateEvent)
				r.Delete("/events/{id}", s.handleDeleteEvent)
				r.Post("/events/{id}/rsvp", s.handleRSVP)
				r.Delete("/events/{id}/rsvp", s.handleCancelRSVP)
				r.Get("/events/{id}/attendees", s.handleAttendees)

				// поиск и жалобы
				r.Get("/search", s.handleSearch)
				r.Post("/reports", s.handleCreateReport)

				// модерация
				r.Route("/mod", func(r chi.Router) {
					r.Use(requireRole("moderator", "admin"))
					r.Get("/reports", s.handleModReports)
					r.Post("/reports/{id}/resolve", s.handleResolveReport)
					r.Post("/users/{id}/ban", s.handleBan)
					r.Post("/users/{id}/unban", s.handleUnban)
					r.Post("/{type}/{id}/hide", s.handleSetHidden(true))
					r.Post("/{type}/{id}/unhide", s.handleSetHidden(false))
					r.Get("/log", s.handleModLog)
				})
				r.Route("/admin", func(r chi.Router) {
					r.Use(requireRole("admin"))
					r.Put("/users/{id}/role", s.handleSetRole)
					r.Post("/wallet/mint", s.handleMint)
					r.Get("/vulnerability-reports", s.handleVulnList)
					r.Patch("/vulnerability-reports/{id}", s.handleVulnStatus)
				})
			})

			// приём сообщений об уязвимостях — без входа в аккаунт
			r.Post("/security/vulnerability-reports", s.handleVulnReport)
		})
	})

	// Загрузка фото: свой лимит тела (10 MiB) и таймаут подлиннее.
	r.With(deadlines(90*time.Second), middleware.Timeout(60*time.Second), s.requireAuth).
		Post("/api/v1/media", s.handleUpload)
	// Загрузка музыки: до 50 МБ, перекодирование до нескольких минут.
	r.With(deadlines(10*time.Minute), middleware.Timeout(9*time.Minute), s.requireAuth).
		Post("/api/v1/tracks", s.handleUploadTrack)
	// Загрузка видео: до 1 ГБ потоком на диск, обработка — в фоне.
	r.With(deadlines(60*time.Minute), s.requireAuth).Post("/api/v1/videos", s.handleUploadVideo)
	// HLS: плейлисты, сегменты, превью.
	r.With(deadlines(10*time.Minute)).Get("/video/{vid}/{file}", s.handleServeVideo)

	// Голосовые сообщения: до 5 МБ.
	r.With(deadlines(5*time.Minute), middleware.Timeout(4*time.Minute), s.requireAuth).
		Post("/api/v1/voice", s.handleUploadVoice)

	// Отдача аудио — публичная, для <audio>, с перемоткой (Range).
	r.With(deadlines(30*time.Minute)).Get("/audio/{file}", s.handleServeAudio)

	// Отдача фото — публичная, для тегов <img>.
	r.With(deadlines(60*time.Second)).Get("/media/{id}/{size}", s.handleServeMedia)

	// WebSocket — долгоживущее соединение: дедлайны снимаем, живость
	// соединения проверяется ping-ами.
	r.With(deadlines(0)).Get("/api/v1/ws", s.handleWS)

	return r
}
