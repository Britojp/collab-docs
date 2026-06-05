package br.ufg.collabdocs.audit;

import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

@SpringBootApplication
public class AuditApplication implements CommandLineRunner {

    public static void main(String[] args) {
        SpringApplication.run(AuditApplication.class, args);
    }

    @Override
    public void run(String... args) throws InterruptedException {
        Thread.currentThread().join();
    }
}
