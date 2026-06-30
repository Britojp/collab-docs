package br.ufg.collabdocs.operation;

public record OpEvent(
        String eventId,
        String docId,
        String userId,
        String timestamp,
        int version,
        String type,
        int pos,
        String character,
        String content) {

    public String normalizedType() {
        return type == null ? "" : type.toLowerCase();
    }
}
