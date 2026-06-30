package br.ufg.collabdocs.document;

import br.ufg.collabdocs.document.dto.CreateDocumentRequest;
import br.ufg.collabdocs.document.dto.DocumentResponse;
import br.ufg.collabdocs.operation.OpEvent;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;
import org.springframework.web.server.ResponseStatusException;

import java.time.LocalDateTime;
import java.util.List;
import java.util.UUID;

@Service
public class DocumentService {

    private final DocumentRepository docs;

    public DocumentService(DocumentRepository docs) {
        this.docs = docs;
    }

    public List<DocumentResponse> listAll() {
        return docs.findAllByOrderByUpdatedAtDesc().stream()
                .map(this::toResponse)
                .toList();
    }

    public DocumentResponse create(CreateDocumentRequest req, UUID ownerId) {
        var doc = new Document();
        doc.setTitle(req.title());
        doc.setOwnerId(ownerId);
        return toResponse(docs.save(doc));
    }

    public DocumentResponse getById(UUID id) {
        return docs.findById(id)
                .map(this::toResponse)
                .orElseThrow(() -> new ResponseStatusException(HttpStatus.NOT_FOUND));
    }

    public String getContent(UUID id) {
        return docs.findById(id)
                .map(Document::getContent)
                .orElseThrow(() -> new ResponseStatusException(HttpStatus.NOT_FOUND));
    }

    public void delete(UUID id, UUID requesterId) {
        var doc = docs.findById(id)
                .orElseThrow(() -> new ResponseStatusException(HttpStatus.NOT_FOUND));
        if (!doc.getOwnerId().equals(requesterId)) {
            throw new ResponseStatusException(HttpStatus.FORBIDDEN);
        }
        docs.delete(doc);
    }

    @Transactional
    public void updateContent(UUID id, String content, int version) {
        docs.findById(id).ifPresent(doc -> {
            doc.setContent(content);
            doc.setVersion(version);
            doc.setUpdatedAt(LocalDateTime.now());
            docs.save(doc);
        });
    }

    @Transactional
    public void applyOperation(OpEvent event) {
        UUID docId = UUID.fromString(event.docId());
        Document doc = docs.findById(docId)
                .orElseThrow(() -> new ResponseStatusException(HttpStatus.NOT_FOUND));

        if (event.version() <= doc.getVersion()) {
            return;
        }
        if (event.version() != doc.getVersion() + 1) {
            if (event.content() == null) {
                throw new IllegalStateException("operation version gap for document " + docId
                        + ": current=" + doc.getVersion() + " event=" + event.version());
            }
            doc.setContent(event.content());
            doc.setVersion(event.version());
            doc.setUpdatedAt(LocalDateTime.now());
            docs.save(doc);
            return;
        }

        String content = doc.getContent() == null ? "" : doc.getContent();
        String updated;
        try {
            updated = switch (event.normalizedType()) {
                case "insert" -> insertAt(content, event.pos(), event.character());
                case "delete" -> deleteAt(content, event.pos());
                default -> throw new IllegalArgumentException("unsupported operation type: " + event.type());
            };
        } catch (IllegalArgumentException e) {
            if (event.content() == null) {
                throw e;
            }
            updated = event.content();
        }

        doc.setContent(updated);
        doc.setVersion(event.version());
        doc.setUpdatedAt(LocalDateTime.now());
        docs.save(doc);
    }

    private String insertAt(String content, int pos, String character) {
        if (character == null || character.isEmpty()) {
            throw new IllegalArgumentException("insert operation requires character");
        }
        int safePos = requirePosition(pos, 0, content.length(), "insert");
        String ch = character.substring(0, 1);
        return content.substring(0, safePos) + ch + content.substring(safePos);
    }

    private String deleteAt(String content, int pos) {
        int safePos = requirePosition(pos, 0, content.length() - 1, "delete");
        return content.substring(0, safePos) + content.substring(safePos + 1);
    }

    private int requirePosition(int pos, int min, int max, String operation) {
        if (pos < min || pos > max) {
            throw new IllegalArgumentException(operation + " position out of bounds: " + pos);
        }
        return pos;
    }

    private DocumentResponse toResponse(Document d) {
        return new DocumentResponse(
                d.getId(), d.getTitle(), d.getOwnerId(), d.getContent(),
                d.getVersion(), d.getCreatedAt(), d.getUpdatedAt());
    }
}
