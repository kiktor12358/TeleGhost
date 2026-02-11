package main

// CopyToClipboard копирует текст.
func (a *App) CopyToClipboard(text string) {
	a.core.CopyToClipboard(text)
}

// GetFileBase64 читает файл.
func (a *App) GetFileBase64(path string) (string, error) {
	return a.core.GetFileBase64(path)
}
