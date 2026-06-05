package br.ufg.collabdocs.user.service;

import br.ufg.collabdocs.user.entity.User;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.test.util.ReflectionTestUtils;

import java.util.UUID;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class JwtServiceTest {

    private JwtService jwtService;

    @BeforeEach
    void setUp() {
        jwtService = new JwtService();
        ReflectionTestUtils.setField(jwtService, "secret", "test-secret-must-be-at-least-32-chars-long");
        ReflectionTestUtils.setField(jwtService, "expirationMs", 86400000L);
    }

    private User user(UUID id, String email, String name) {
        User u = new User();
        ReflectionTestUtils.setField(u, "id", id);
        u.setEmail(email);
        u.setName(name);
        u.setPasswordHash("hash");
        return u;
    }

    @Test
    void generateToken_returnsNonBlankToken() {
        User u = user(UUID.randomUUID(), "a@b.com", "Alice");
        assertThat(jwtService.generateToken(u)).isNotBlank();
    }

    @Test
    void extractUserId_returnsCorrectSubject() {
        UUID id = UUID.randomUUID();
        User u = user(id, "a@b.com", "Alice");
        String token = jwtService.generateToken(u);
        assertThat(jwtService.extractUserId(token)).isEqualTo(id.toString());
    }

    @Test
    void isValid_trueForFreshToken() {
        User u = user(UUID.randomUUID(), "a@b.com", "Alice");
        assertThat(jwtService.isValid(jwtService.generateToken(u))).isTrue();
    }

    @Test
    void isValid_falseForTamperedToken() {
        User u = user(UUID.randomUUID(), "a@b.com", "Alice");
        String token = jwtService.generateToken(u) + "tampered";
        assertThat(jwtService.isValid(token)).isFalse();
    }

    @Test
    void isValid_falseForExpiredToken() {
        JwtService expiredService = new JwtService();
        ReflectionTestUtils.setField(expiredService, "secret", "test-secret-must-be-at-least-32-chars-long");
        ReflectionTestUtils.setField(expiredService, "expirationMs", -1000L); // já expirado

        User u = user(UUID.randomUUID(), "a@b.com", "Alice");
        String token = expiredService.generateToken(u);
        assertThat(expiredService.isValid(token)).isFalse();
    }
}
