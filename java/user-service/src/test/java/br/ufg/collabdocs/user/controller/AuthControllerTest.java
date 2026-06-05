package br.ufg.collabdocs.user.controller;

import br.ufg.collabdocs.user.dto.AuthResponse;
import br.ufg.collabdocs.user.service.UserService;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.boot.test.mock.mockito.MockBean;
import org.springframework.http.MediaType;
import org.springframework.security.authentication.BadCredentialsException;
import org.springframework.test.web.servlet.MockMvc;

import java.util.UUID;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.*;

@WebMvcTest(AuthController.class)
class AuthControllerTest {

    @Autowired MockMvc mockMvc;
    @Autowired ObjectMapper objectMapper;
    @MockBean  UserService userService;

    private static final AuthResponse FAKE_RESPONSE =
            new AuthResponse("jwt-token", UUID.randomUUID(), "Alice", "a@b.com");

    @Test
    void register_returns201WithToken() throws Exception {
        when(userService.register(any())).thenReturn(FAKE_RESPONSE);

        mockMvc.perform(post("/auth/register")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("""
                                {"name":"Alice","email":"a@b.com","password":"pass1234"}
                                """))
                .andExpect(status().isCreated())
                .andExpect(jsonPath("$.token").value("jwt-token"))
                .andExpect(jsonPath("$.email").value("a@b.com"));
    }

    @Test
    void register_returns400ForInvalidPayload() throws Exception {
        mockMvc.perform(post("/auth/register")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("""
                                {"name":"","email":"not-an-email","password":"short"}
                                """))
                .andExpect(status().isBadRequest());
    }

    @Test
    void login_returns200WithToken() throws Exception {
        when(userService.login(any())).thenReturn(FAKE_RESPONSE);

        mockMvc.perform(post("/auth/login")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("""
                                {"email":"a@b.com","password":"pass1234"}
                                """))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.token").value("jwt-token"));
    }

    @Test
    void login_returns401ForBadCredentials() throws Exception {
        when(userService.login(any())).thenThrow(new BadCredentialsException("Invalid credentials"));

        mockMvc.perform(post("/auth/login")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("""
                                {"email":"a@b.com","password":"wrong"}
                                """))
                .andExpect(status().isUnauthorized());
    }

    @Test
    void login_returns400ForMissingFields() throws Exception {
        mockMvc.perform(post("/auth/login")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content("{}"))
                .andExpect(status().isBadRequest());
    }
}
