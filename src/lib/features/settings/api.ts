import { invoke } from "@tauri-apps/api/core";
import type { AppSettings, DocumentPreviewPosition, FontSize } from "./types";

function inTauriRuntime() {
  return typeof window !== "undefined" && "__TAURI_INTERNALS__" in window;
}

function fallbackSettings(overrides: Partial<AppSettings> = {}): AppSettings {
  return {
    fontSize: "standard",
    fontFamily: "",
    documentPreviewPosition: "bottom",
    documentPreviewRatio: 0.5,
    ...overrides,
  };
}

export const settingsApi = {
  get() {
    if (!inTauriRuntime()) return Promise.resolve(fallbackSettings());
    return invoke<AppSettings>("get_app_settings");
  },
  setFontSize(fontSize: FontSize) {
    if (!inTauriRuntime()) return Promise.resolve(fallbackSettings({ fontSize }));
    return invoke<AppSettings>("set_font_size", { fontSize });
  },
  setFontFamily(fontFamily: string) {
    if (!inTauriRuntime()) return Promise.resolve(fallbackSettings({ fontFamily }));
    return invoke<AppSettings>("set_font_family", { fontFamily });
  },
  setDocumentPreviewLayout(position: DocumentPreviewPosition, ratio: number) {
    if (!inTauriRuntime()) {
      return Promise.resolve(fallbackSettings({
        documentPreviewPosition: position,
        documentPreviewRatio: ratio,
      }));
    }
    return invoke<AppSettings>("set_document_preview_layout", { position, ratio });
  },
  systemFonts() {
    if (!inTauriRuntime()) {
      return Promise.resolve(["Arial", "Georgia", "Helvetica Neue", "Menlo", "Times New Roman"]);
    }
    return invoke<string[]>("get_system_fonts");
  },
};
