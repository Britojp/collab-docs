package br.ufg.collabdocs.analytics;

import br.ufg.collabdocs.document.Document;
import br.ufg.collabdocs.document.DocumentRepository;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Service;
import org.springframework.web.server.ResponseStatusException;

import java.util.Arrays;
import java.util.UUID;
import java.util.regex.Pattern;

@Service
public class AnalyticsService {

    private static final Pattern WORD_SEPARATOR = Pattern.compile("\\s+");
    private static final Pattern PARAGRAPH_SEPARATOR = Pattern.compile("(\\R\\s*){2,}");

    private final DocumentRepository documents;

    public AnalyticsService(DocumentRepository documents) {
        this.documents = documents;
    }

    public DocumentAnalytics getDocumentAnalytics(UUID docId) {
        Document doc = documents.findById(docId)
                .orElseThrow(() -> new ResponseStatusException(HttpStatus.NOT_FOUND));

        String content = doc.getContent() == null ? "" : doc.getContent();
        return new DocumentAnalytics(
                doc.getId(),
                content.length(),
                countWords(content),
                countLines(content),
                countParagraphs(content),
                doc.getVersion(),
                doc.getUpdatedAt());
    }

    private long countWords(String content) {
        String trimmed = content.trim();
        if (trimmed.isEmpty()) {
            return 0;
        }
        return WORD_SEPARATOR.splitAsStream(trimmed).count();
    }

    private long countLines(String content) {
        if (content.isEmpty()) {
            return 0;
        }
        long lines = 1;
        for (int i = 0; i < content.length(); i++) {
            char current = content.charAt(i);
            if (current == '\n') {
                lines++;
            } else if (current == '\r') {
                lines++;
                if (i + 1 < content.length() && content.charAt(i + 1) == '\n') {
                    i++;
                }
            }
        }
        return lines;
    }

    private long countParagraphs(String content) {
        String trimmed = content.trim();
        if (trimmed.isEmpty()) {
            return 0;
        }
        return Arrays.stream(PARAGRAPH_SEPARATOR.split(trimmed))
                .filter(part -> !part.trim().isEmpty())
                .count();
    }
}
