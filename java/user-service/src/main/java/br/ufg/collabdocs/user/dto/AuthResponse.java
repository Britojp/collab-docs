package br.ufg.collabdocs.user.dto;

import java.util.UUID;

public record AuthResponse(
    String token,
    UUID userId,
    String name,
    String email
) {}
