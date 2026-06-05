package br.ufg.collabdocs.notif;

import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

@SpringBootApplication
public class NotifApplication implements CommandLineRunner {

    public static void main(String[] args) {
        SpringApplication.run(NotifApplication.class, args);
    }

    @Override
    public void run(String... args) throws InterruptedException {
        Thread.currentThread().join();
    }
}
