export type FontSize = "extra-small" | "small" | "standard" | "large" | "extra-large";
export type DocumentPreviewPosition = "right" | "bottom";

export type AppSettings = {
  fontSize: FontSize;
  fontFamily: string;
  documentPreviewPosition: DocumentPreviewPosition;
  documentPreviewRatio: number;
};
