package br.ufg.collabdocs.operation;

import jakarta.persistence.*;
import java.time.LocalDateTime;
import java.util.UUID;

@Entity
@Table(name = "operations")
public class Operation {

    @Id
    @GeneratedValue(strategy = GenerationType.UUID)
    private UUID id;

    @Column(name = "doc_id", nullable = false)
    private UUID docId;

    @Column(name = "user_id")
    private UUID userId;

    private String type;
    private int position;

    @Column(name = "character", length = 1)
    private String character;

    @Column(name = "server_version")
    private int serverVersion;

    @Column(name = "created_at")
    private LocalDateTime createdAt = LocalDateTime.now();

    public static Operation from(OpEvent ev) {
        var op = new Operation();
        op.docId = UUID.fromString(ev.docId());
        try { op.userId = UUID.fromString(ev.userId()); } catch (Exception ignored) {}
        op.type = ev.normalizedType();
        op.position = ev.pos();
        op.character = ev.character();
        op.serverVersion = ev.version();
        return op;
    }

    public UUID getId()          { return id; }
    public UUID getDocId()       { return docId; }
    public UUID getUserId()      { return userId; }
    public String getType()      { return type; }
    public int getPosition()     { return position; }
    public String getCharacter() { return character; }
    public int getServerVersion(){ return serverVersion; }
    public LocalDateTime getCreatedAt() { return createdAt; }
}
