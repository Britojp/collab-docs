package br.ufg.collabdocs.document;

import br.ufg.collabdocs.document.dto.CreateDocumentRequest;
import br.ufg.collabdocs.document.dto.DocumentResponse;
import org.springframework.http.HttpStatus;
import org.springframework.web.bind.annotation.*;

import java.util.List;
import java.util.Map;
import java.util.UUID;

@RestController
public class DocumentController {

    private final DocumentService service;

    public DocumentController(DocumentService service) {
        this.service = service;
    }

    @GetMapping("/documents")
    public List<DocumentResponse> list() {
        return service.listAll();
    }

    @PostMapping("/documents")
    @ResponseStatus(HttpStatus.CREATED)
    public DocumentResponse create(
            @RequestBody CreateDocumentRequest req,
            @RequestHeader("X-User-ID") String userId) {
        return service.create(req, UUID.fromString(userId));
    }

    @GetMapping("/documents/{id}")
    public DocumentResponse get(@PathVariable UUID id) {
        return service.getById(id);
    }

    @DeleteMapping("/documents/{id}")
    @ResponseStatus(HttpStatus.NO_CONTENT)
    public void delete(
            @PathVariable UUID id,
            @RequestHeader("X-User-ID") String userId) {
        service.delete(id, UUID.fromString(userId));
    }

    // Internal endpoint called by the Go Hub Manager to load document content on hub creation.
    @GetMapping("/internal/documents/{id}/content")
    public Map<String, Object> getContent(@PathVariable UUID id) {
        DocumentResponse doc = service.getById(id);
        return Map.of(
                "content", doc.content(),
                "version", doc.version());
    }
}
