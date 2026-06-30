package br.ufg.collabdocs.analytics;

import java.time.LocalDateTime;
import java.util.UUID;

public record DocumentAnalytics(
        UUID docId,
        long charCount,
        long wordCount,
        long lineCount,
        long paragraphCount,
        int version,
        LocalDateTime lastActivity) {
}
