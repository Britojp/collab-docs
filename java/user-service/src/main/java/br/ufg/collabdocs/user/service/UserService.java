package br.ufg.collabdocs.user.service;

import br.ufg.collabdocs.user.dto.*;
import br.ufg.collabdocs.user.entity.User;
import br.ufg.collabdocs.user.repository.UserRepository;
import org.springframework.security.authentication.BadCredentialsException;
import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.stereotype.Service;

import java.util.UUID;

@Service
public class UserService {

    private final UserRepository users;
    private final PasswordEncoder encoder;
    private final JwtService jwt;

    public UserService(UserRepository users, PasswordEncoder encoder, JwtService jwt) {
        this.users = users;
        this.encoder = encoder;
        this.jwt = jwt;
    }

    public AuthResponse register(RegisterRequest req) {
        if (users.existsByEmail(req.email())) {
            throw new IllegalArgumentException("Email already in use");
        }
        User user = new User();
        user.setEmail(req.email());
        user.setName(req.name());
        user.setPasswordHash(encoder.encode(req.password()));
        users.save(user);
        return new AuthResponse(jwt.generateToken(user), user.getId(), user.getName(), user.getEmail());
    }

    public AuthResponse login(LoginRequest req) {
        User user = users.findByEmail(req.email())
                .orElseThrow(() -> new BadCredentialsException("Invalid credentials"));
        if (!encoder.matches(req.password(), user.getPasswordHash())) {
            throw new BadCredentialsException("Invalid credentials");
        }
        return new AuthResponse(jwt.generateToken(user), user.getId(), user.getName(), user.getEmail());
    }

    public UserResponse findById(UUID id) {
        return users.findById(id)
                .map(UserResponse::from)
                .orElseThrow(() -> new IllegalArgumentException("User not found"));
    }

    public UserResponse update(UUID id, RegisterRequest req) {
        User user = users.findById(id)
                .orElseThrow(() -> new IllegalArgumentException("User not found"));
        user.setName(req.name());
        if (req.password() != null && !req.password().isBlank()) {
            user.setPasswordHash(encoder.encode(req.password()));
        }
        return UserResponse.from(users.save(user));
    }

    public void delete(UUID id) {
        users.deleteById(id);
    }
}
