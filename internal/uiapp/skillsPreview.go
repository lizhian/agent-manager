package uiapp

// SetPreview applies the shared document preference without writing it again.
func (w *SkillsWorkspace) SetPreview(position string, ratio float64, onChange func(string, float64)) {
	w.bottom = position == "bottom"
	w.previewPercent = float32((1 - ratio) * 100)
	w.previewChanged = onChange
}
func (w *SkillsWorkspace) notifyPreview() {
	if w.previewChanged != nil {
		position := "right"
		if w.bottom {
			position = "bottom"
		}
		w.previewChanged(position, 1-float64(w.previewPercent)/100)
	}
}
