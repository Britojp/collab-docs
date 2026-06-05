package br.ufg.collabdocs.user.controller;

import br.ufg.collabdocs.user.dto.AuthResponse;
import br.ufg.collabdocs.user.dto.LoginRequest;
import br.ufg.collabdocs.user.dto.RegisterRequest;
import br.ufg.collabdocs.user.service.UserService;
import jakarta.validation.Valid;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/auth")
public class AuthController {

    private final UserService userService;

    public AuthController(UserService userService) {
        this.userService = userService;
    }

    @PostMapping("/register")
    public ResponseEntity<AuthResponse> register(@Valid @RequestBody RegisterRequest req) {
        return ResponseEntity.status(HttpStatus.CREATED).body(userService.register(req));
    }

    @PostMapping("/login")
    public ResponseEntity<AuthResponse> login(@Valid @RequestBody LoginRequest req) {
        return ResponseEntity.ok(userService.login(req));
    }
}
