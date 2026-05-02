using Npgsql;

var builder = WebApplication.CreateBuilder(args);
var app = builder.Build();

app.MapGet("/catalog", async () =>
{
    var connectionString = "Host=catalog-db;Username=postgres;Password=password;Database=catalog";
    
    try
    {
        // Try to connect to the database with a short timeout
        // (Timeout logic usually managed by connection string or cancellation token)
        await using var dataSource = NpgsqlDataSource.Create(connectionString);
        await using var connection = await dataSource.OpenConnectionAsync();
        
        await using var command = new NpgsqlCommand("SELECT 'Real Product from DB' as Name", connection);
        await using var reader = await command.ExecuteReaderAsync();
        
        if (await reader.ReadAsync())
        {
            return Results.Ok(new { source = "postgresql", data = reader.GetString(0) });
        }
        return Results.Ok(new { source = "postgresql", data = "Empty DB" });
    }
    catch (Exception ex)
    {
        Console.WriteLine($"Database connection failed: {ex.Message}. Falling back to degraded mode.");
        
        // Resilience: Graceful Degradation. 
        // Database is unreachable, but we still return a 200 OK with local fallback data 
        // instead of crashing the whole service.
        var fallbackData = new[] { 
            new { id = 1, name = "Fallback Item 1 (DB Down)" },
            new { id = 2, name = "Fallback Item 2 (DB Down)" }
        };
        
        return Results.Ok(new { source = "local-fallback", data = fallbackData, status = "degraded" });
    }
});

app.Run("http://*:80");
