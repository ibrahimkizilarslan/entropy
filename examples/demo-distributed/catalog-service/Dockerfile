FROM mcr.microsoft.com/dotnet/sdk:8.0 AS build
WORKDIR /src
COPY catalog-service.csproj .
RUN dotnet restore
COPY . .
RUN dotnet publish -c Release -o /app

FROM mcr.microsoft.com/dotnet/aspnet:8.0
WORKDIR /app
COPY --from=build /app .
EXPOSE 80
ENTRYPOINT ["dotnet", "catalog-service.dll"]
