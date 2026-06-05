package br.ufg.collabdocs.user.dto;

import jakarta.validation.constraints.Email;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.Size;

public record RegisterRequest(
    @NotBlank @Size(min = 2, max = 100) String name,
    @Email @NotBlank String email,
    @NotBlank @Size(min = 8) String password
) {}
