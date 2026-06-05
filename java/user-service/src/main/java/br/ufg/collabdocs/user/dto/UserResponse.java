package br.ufg.collabdocs.user.dto;

import br.ufg.collabdocs.user.entity.User;

import java.time.OffsetDateTime;
import java.util.UUID;

public record UserResponse(
    UUID id,
    String name,
    String email,
    OffsetDateTime createdAt
) {
    public static UserResponse from(User u) {
        return new UserResponse(u.getId(), u.getName(), u.getEmail(), u.getCreatedAt());
    }
}
