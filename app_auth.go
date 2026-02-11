package main

// ListProfiles возвращает список доступных профилей.
func (a *App) ListProfiles() ([]map[string]interface{}, error) {
	return a.core.ListProfiles()
}

// CreateProfile создаёт новый профиль.
func (a *App) CreateProfile(name, pin, mnemonic, userID, avatarPath string, usePin bool) error {
	return a.core.CreateProfile(name, pin, mnemonic, userID, avatarPath, usePin)
}

// UnlockProfile проверяет PIN и возвращает мнемонику.
func (a *App) UnlockProfile(profileID, pin string) (string, error) {
	return a.core.UnlockProfile(profileID, pin)
}

// DeleteProfile удаляет профиль.
func (a *App) DeleteProfile(profileID string) error {
	return a.core.DeleteProfile(profileID)
}

// Login авторизует пользователя.
func (a *App) Login(seedPhrase string) error {
	return a.core.Login(seedPhrase)
}

// CreateAccount создаёт новый аккаунт.
func (a *App) CreateAccount() (string, error) {
	return a.core.CreateAccount()
}

// Logout выходит из аккаунта.
func (a *App) Logout() {
	a.core.Logout()
}

// GetMyInfo возвращает информацию о текущем пользователе.
func (a *App) GetMyInfo() map[string]interface{} {
	return a.core.GetMyInfo()
}

// UpdateMyProfile обновляет профиль пользователя.
func (a *App) UpdateMyProfile(nickname, bio, avatar string) error {
	return a.core.UpdateMyProfile(nickname, bio, avatar)
}

// GetCurrentProfile возвращает текущий профиль.
func (a *App) GetCurrentProfile() map[string]interface{} {
	return a.core.GetCurrentProfile()
}

// UpdateProfile обновляет данные профиля (ПИН-код и т.д.)
func (a *App) UpdateProfile(profileID, name, avatarPath string, deleteAvatar bool, usePin bool, newPin, mnemonic string) error {
	return a.core.UpdateProfile(profileID, name, avatarPath, deleteAvatar, usePin, newPin, mnemonic)
}
