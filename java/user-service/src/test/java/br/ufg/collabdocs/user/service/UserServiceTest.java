package br.ufg.collabdocs.user.service;

import br.ufg.collabdocs.user.dto.AuthResponse;
import br.ufg.collabdocs.user.dto.LoginRequest;
import br.ufg.collabdocs.user.dto.RegisterRequest;
import br.ufg.collabdocs.user.dto.UserResponse;
import br.ufg.collabdocs.user.entity.User;
import br.ufg.collabdocs.user.repository.UserRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;
import org.springframework.security.authentication.BadCredentialsException;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.test.util.ReflectionTestUtils;

import java.util.Optional;
import java.util.UUID;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.*;

@ExtendWith(MockitoExtension.class)
class UserServiceTest {

    @Mock UserRepository userRepository;
    @Mock PasswordEncoder passwordEncoder;
    @Mock JwtService jwtService;

    @InjectMocks UserService userService;

    private User fakeUser(UUID id, String email) {
        User u = new User();
        ReflectionTestUtils.setField(u, "id", id);
        u.setEmail(email);
        u.setName("Alice");
        u.setPasswordHash("hash");
        return u;
    }

    @Test
    void register_savesUserAndReturnsToken() {
        when(userRepository.existsByEmail("a@b.com")).thenReturn(false);
        when(passwordEncoder.encode("pass1234")).thenReturn("hash");
        when(userRepository.save(any())).thenAnswer(inv -> {
            User u = inv.getArgument(0);
            ReflectionTestUtils.setField(u, "id", UUID.randomUUID());
            return u;
        });
        when(jwtService.generateToken(any())).thenReturn("jwt-token");

        AuthResponse resp = userService.register(new RegisterRequest("Alice", "a@b.com", "pass1234"));

        assertThat(resp.token()).isEqualTo("jwt-token");
        assertThat(resp.email()).isEqualTo("a@b.com");
        verify(userRepository).save(any());
    }

    @Test
    void register_throwsWhenEmailInUse() {
        when(userRepository.existsByEmail("a@b.com")).thenReturn(true);

        assertThatThrownBy(() -> userService.register(new RegisterRequest("Alice", "a@b.com", "pass1234")))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("Email already in use");
    }

    @Test
    void login_returnsTokenForValidCredentials() {
        UUID id = UUID.randomUUID();
        User u = fakeUser(id, "a@b.com");
        when(userRepository.findByEmail("a@b.com")).thenReturn(Optional.of(u));
        when(passwordEncoder.matches("pass1234", "hash")).thenReturn(true);
        when(jwtService.generateToken(u)).thenReturn("jwt-token");

        AuthResponse resp = userService.login(new LoginRequest("a@b.com", "pass1234"));

        assertThat(resp.token()).isEqualTo("jwt-token");
    }

    @Test
    void login_throwsForUnknownEmail() {
        when(userRepository.findByEmail("x@b.com")).thenReturn(Optional.empty());

        assertThatThrownBy(() -> userService.login(new LoginRequest("x@b.com", "pass")))
                .isInstanceOf(BadCredentialsException.class);
    }

    @Test
    void login_throwsForWrongPassword() {
        User u = fakeUser(UUID.randomUUID(), "a@b.com");
        when(userRepository.findByEmail("a@b.com")).thenReturn(Optional.of(u));
        when(passwordEncoder.matches("wrong", "hash")).thenReturn(false);

        assertThatThrownBy(() -> userService.login(new LoginRequest("a@b.com", "wrong")))
                .isInstanceOf(BadCredentialsException.class);
    }

    @Test
    void findById_returnsUserResponse() {
        UUID id = UUID.randomUUID();
        when(userRepository.findById(id)).thenReturn(Optional.of(fakeUser(id, "a@b.com")));

        UserResponse resp = userService.findById(id);

        assertThat(resp.id()).isEqualTo(id);
        assertThat(resp.email()).isEqualTo("a@b.com");
    }

    @Test
    void findById_throwsWhenNotFound() {
        UUID id = UUID.randomUUID();
        when(userRepository.findById(id)).thenReturn(Optional.empty());

        assertThatThrownBy(() -> userService.findById(id))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("User not found");
    }

    @Test
    void delete_callsRepository() {
        UUID id = UUID.randomUUID();
        userService.delete(id);
        verify(userRepository).deleteById(id);
    }
}
